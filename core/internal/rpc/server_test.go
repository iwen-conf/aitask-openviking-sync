package rpc

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	aitaskv1 "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/rpc/gen/aitask/v1"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/rpc/gen/aitask/v1/aitaskv1connect"
)

func TestWhoAmIConnectProtocol(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewHandler(ServerDeps{}))
	defer srv.Close()

	client := aitaskv1connect.NewAgentServiceClient(srv.Client(), srv.URL)
	_, err := client.WhoAmI(context.Background(), connect.NewRequest(&aitaskv1.WhoAmIRequest{}))
	assertAppError(t, err, connect.CodePermissionDenied, "PROJECT_ACCESS_DENIED", "false")
}

func TestWhoAmIGRPCProtocol(t *testing.T) {
	t.Parallel()

	srv := httptest.NewUnstartedServer(NewHandler(ServerDeps{}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	client := aitaskv1connect.NewAgentServiceClient(srv.Client(), srv.URL, connect.WithGRPC())
	_, err := client.WhoAmI(context.Background(), connect.NewRequest(&aitaskv1.WhoAmIRequest{}))
	assertAppError(t, err, connect.CodePermissionDenied, "PROJECT_ACCESS_DENIED", "false")
}

func TestServiceRoutesReachable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewHandler(ServerDeps{}))
	defer srv.Close()

	bootstrapClient := aitaskv1connect.NewBootstrapServiceClient(srv.Client(), srv.URL)
	_, bootstrapErr := bootstrapClient.Bootstrap(context.Background(), connect.NewRequest(&aitaskv1.BootstrapRequest{}))
	assertAppError(t, bootstrapErr, connect.CodeInvalidArgument, "INVALID_ARGUMENT", "false")

	contextClient := aitaskv1connect.NewContextServiceClient(srv.Client(), srv.URL)
	_, reportErr := contextClient.Report(context.Background(), connect.NewRequest(&aitaskv1.ReportRequest{}))
	assertAppError(t, reportErr, connect.CodeUnavailable, "INTERNAL", "true")

	taskClient := aitaskv1connect.NewTaskServiceClient(srv.Client(), srv.URL)
	_, taskErr := taskClient.GetCurrentTask(context.Background(), connect.NewRequest(&aitaskv1.GetCurrentTaskRequest{
		ProjectId: "prj_01TESTPROJECT",
	}))
	assertAppError(t, taskErr, connect.CodePermissionDenied, "PROJECT_ACCESS_DENIED", "false")
}

func assertAppError(t *testing.T, err error, wantConnectCode connect.Code, wantAppCode string, wantRetriable string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("expected connect error, got %T", err)
	}
	if got := connect.CodeOf(connectErr); got != wantConnectCode {
		t.Fatalf("unexpected connect code: got=%v want=%v", got, wantConnectCode)
	}
	if got := connectErr.Meta().Get("x-aitask-code"); got != wantAppCode {
		t.Fatalf("unexpected app code metadata: got=%q want=%q", got, wantAppCode)
	}
	if got := connectErr.Meta().Get("x-aitask-retriable"); got != wantRetriable {
		t.Fatalf("unexpected retriable metadata: got=%q want=%q", got, wantRetriable)
	}
}
