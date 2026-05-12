package health

import (
	"context"
	"errors"
	"testing"
)

func TestReadinessReady(t *testing.T) {
	service := NewReadiness(ReadinessOptions{
		Dependencies: []Dependency{
			{
				Name:     "postgres",
				Critical: true,
				Check: func(context.Context) error {
					return nil
				},
			},
			{
				Name:     "dragonfly",
				Critical: true,
				Check: func(context.Context) error {
					return nil
				},
			},
		},
	})

	snapshot := service.Check(context.Background())

	if got, want := snapshot.Status, ReadyStatusReady; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

func TestReadinessDegraded(t *testing.T) {
	service := NewReadiness(ReadinessOptions{
		Dependencies: []Dependency{
			{
				Name:     "postgres",
				Critical: true,
				Check: func(context.Context) error {
					return nil
				},
			},
			{
				Name:     "openviking",
				Critical: false,
				Check: func(context.Context) error {
					return errors.New("unavailable")
				},
			},
		},
	})

	snapshot := service.Check(context.Background())

	if got, want := snapshot.Status, ReadyStatusDegraded; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := snapshot.Dependencies["openviking"], DependencyStatusUnavailable; got != want {
		t.Fatalf("dependencies[openviking] = %q, want %q", got, want)
	}
}

func TestReadinessNotReady(t *testing.T) {
	service := NewReadiness(ReadinessOptions{
		Dependencies: []Dependency{
			{
				Name:     "postgres",
				Critical: true,
				Check: func(context.Context) error {
					return errors.New("connection refused")
				},
			},
			{
				Name:     "dragonfly",
				Critical: true,
				Check: func(context.Context) error {
					return nil
				},
			},
		},
	})

	snapshot := service.Check(context.Background())

	if got, want := snapshot.Status, ReadyStatusNotReady; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := snapshot.Dependencies["postgres"], DependencyStatusUnavailable; got != want {
		t.Fatalf("dependencies[postgres] = %q, want %q", got, want)
	}
}
