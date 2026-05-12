package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/websocket"

	aitaskv1 "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/rpc/gen/aitask/v1"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/rpc/gen/aitask/v1/aitaskv1connect"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: go run ./cmd/review-perf-helper <rpc-p99|ws-p99> ...")
	}
	switch os.Args[1] {
	case "rpc-p99":
		runRPCP99(os.Args[2:])
	case "ws-p99":
		runWSP99(os.Args[2:])
	default:
		fail("unknown subcommand: %s", os.Args[1])
	}
}

func runRPCP99(args []string) {
	fs := flag.NewFlagSet("rpc-p99", flag.ExitOnError)
	server := fs.String("server", "http://127.0.0.1:18080", "server URL")
	projectID := fs.String("project", "", "project ID")
	token := fs.String("token", "", "bearer token")
	concurrency := fs.Int("concurrency", 20, "parallel workers")
	requests := fs.Int("requests", 200, "total requests")
	timeout := fs.Duration("timeout", 15*time.Second, "request timeout")
	fs.Parse(args)

	if strings.TrimSpace(*projectID) == "" || strings.TrimSpace(*token) == "" {
		fail("project and token are required")
	}

	client := aitaskv1connect.NewTaskServiceClient(&http.Client{Timeout: *timeout}, strings.TrimRight(*server, "/"))
	durations := make([]float64, 0, *requests)
	var mu sync.Mutex
	var wg sync.WaitGroup
	work := make(chan struct{}, *requests)
	for i := 0; i < *requests; i++ {
		work <- struct{}{}
	}
	close(work)

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range work {
				req := connect.NewRequest(&aitaskv1.GetCurrentTaskRequest{ProjectId: strings.TrimSpace(*projectID)})
				req.Header().Set("Authorization", "Bearer "+strings.TrimSpace(*token))
				start := time.Now()
				_, err := client.GetCurrentTask(context.Background(), req)
				elapsed := time.Since(start).Seconds() * 1000
				if err != nil {
					fail("rpc request failed: %v", err)
				}
				mu.Lock()
				durations = append(durations, elapsed)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	emitStats("rpc", durations)
}

func runWSP99(args []string) {
	fs := flag.NewFlagSet("ws-p99", flag.ExitOnError)
	server := fs.String("server", "http://127.0.0.1:18080", "server URL")
	projectID := fs.String("project", "", "project ID")
	token := fs.String("token", "", "bearer token")
	connections := fs.Int("connections", 50, "websocket connections")
	warmup := fs.Duration("warmup", 3*time.Second, "warmup duration")
	fs.Parse(args)

	if strings.TrimSpace(*projectID) == "" || strings.TrimSpace(*token) == "" {
		fail("project and token are required")
	}

	wsURL := mustWSURL(*server, *projectID)
	var wg sync.WaitGroup
	latencies := make([]float64, 0, *connections)
	var mu sync.Mutex
	start := time.Now()
	for i := 0; i < *connections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			header := http.Header{}
			header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))
			begin := time.Now()
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
			if err != nil {
				fail("ws dial failed: %v", err)
			}
			defer conn.Close()
			_, _, err = conn.ReadMessage()
			if err != nil {
				fail("ws read failed: %v", err)
			}
			elapsed := time.Since(begin).Seconds() * 1000
			mu.Lock()
			latencies = append(latencies, elapsed)
			mu.Unlock()
		}()
	}
	wg.Wait()
	time.Sleep(*warmup)
	emitStatsWithMeta("ws", latencies, map[string]any{
		"connectedMs": time.Since(start).Milliseconds(),
		"connections": *connections,
	})
}

func mustWSURL(server string, projectID string) string {
	u, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil {
		fail("invalid server URL: %v", err)
	}
	if strings.EqualFold(u.Scheme, "https") {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = path.Join(u.Path, "/ws/projects/", strings.TrimSpace(projectID), "agent-room")
	return u.String()
}

func emitStats(kind string, samples []float64) {
	emitStatsWithMeta(kind, samples, nil)
}

func emitStatsWithMeta(kind string, samples []float64, meta map[string]any) {
	if len(samples) == 0 {
		fail("%s: no samples", kind)
	}
	sort.Float64s(samples)
	sum := 0.0
	for _, sample := range samples {
		sum += sample
	}
	payload := map[string]any{
		"kind":    kind,
		"count":   len(samples),
		"avgMs":   sum / float64(len(samples)),
		"p50Ms":   percentile(samples, 0.50),
		"p95Ms":   percentile(samples, 0.95),
		"p99Ms":   percentile(samples, 0.99),
		"minMs":   samples[0],
		"maxMs":   samples[len(samples)-1],
		"samples": samples,
	}
	for k, v := range meta {
		payload[k] = v
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fail("marshal stats failed: %v", err)
	}
	fmt.Println(string(raw))
}

func percentile(samples []float64, p float64) float64 {
	if len(samples) == 1 {
		return samples[0]
	}
	pos := int(float64(len(samples)-1) * p)
	if pos < 0 {
		pos = 0
	}
	if pos >= len(samples) {
		pos = len(samples) - 1
	}
	return samples[pos]
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
