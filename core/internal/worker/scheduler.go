package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type JobFunc func(ctx context.Context) error

type SchedulerOptions struct {
	Logger *slog.Logger
}

type Scheduler struct {
	logger *slog.Logger
	cron   *cron.Cron

	baseCtx context.Context
	cancel  context.CancelFunc

	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	delayedMu sync.Mutex
	delayed   map[*time.Timer]struct{}
}

func NewScheduler(opts SchedulerOptions) *Scheduler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	baseCtx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		logger:  logger,
		cron:    cron.New(cron.WithSeconds()),
		baseCtx: baseCtx,
		cancel:  cancel,
		delayed: map[*time.Timer]struct{}{},
	}
}

func (s *Scheduler) ScheduleEvery(interval time.Duration, name string, job JobFunc) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	spec := "@every " + interval.String()
	return s.ScheduleCron(spec, name, job)
}

func (s *Scheduler) ScheduleCron(spec string, name string, job JobFunc) error {
	spec = strings.TrimSpace(spec)
	name = strings.TrimSpace(name)
	if spec == "" {
		return fmt.Errorf("cron spec cannot be empty")
	}
	if name == "" {
		return fmt.Errorf("job name cannot be empty")
	}
	if job == nil {
		return fmt.Errorf("job func cannot be nil")
	}
	_, err := s.cron.AddFunc(spec, func() {
		s.runJob(name, job)
	})
	if err != nil {
		return fmt.Errorf("register cron job %q failed: %w", name, err)
	}
	return nil
}

func (s *Scheduler) ScheduleDelayed(delay time.Duration, name string, job JobFunc) (func(), error) {
	if delay < 0 {
		return nil, fmt.Errorf("delay cannot be negative")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("job name cannot be empty")
	}
	if job == nil {
		return nil, fmt.Errorf("job func cannot be nil")
	}

	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		s.removeDelayed(timer)
		s.runJob(name, job)
	})
	s.delayedMu.Lock()
	s.delayed[timer] = struct{}{}
	s.delayedMu.Unlock()

	cancel := func() {
		if timer.Stop() {
			s.removeDelayed(timer)
		}
	}
	return cancel, nil
}

func (s *Scheduler) Start() {
	s.startOnce.Do(func() {
		s.cron.Start()
	})
}

func (s *Scheduler) Shutdown(ctx context.Context) error {
	var shutdownErr error
	s.stopOnce.Do(func() {
		s.cancel()

		cronDone := s.cron.Stop().Done()
		select {
		case <-ctx.Done():
			shutdownErr = ctx.Err()
			return
		case <-cronDone:
		}

		s.delayedMu.Lock()
		for timer := range s.delayed {
			timer.Stop()
			delete(s.delayed, timer)
		}
		s.delayedMu.Unlock()

		waitDone := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(waitDone)
		}()
		select {
		case <-ctx.Done():
			shutdownErr = ctx.Err()
		case <-waitDone:
		}
	})
	return shutdownErr
}

func (s *Scheduler) runJob(name string, job JobFunc) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		start := time.Now()
		err := job(s.baseCtx)
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Warn("worker job failed", "job", name, "error", err)
			return
		}
		s.logger.Debug("worker job finished", "job", name, "duration", time.Since(start).String())
	}()
}

func (s *Scheduler) removeDelayed(timer *time.Timer) {
	if timer == nil {
		return
	}
	s.delayedMu.Lock()
	delete(s.delayed, timer)
	s.delayedMu.Unlock()
}
