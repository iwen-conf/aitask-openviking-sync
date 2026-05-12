package health

import (
	"context"
	"time"
)

const (
	StatusOK   = "ok"
	TimeFormat = time.RFC3339
)

type Clock func() time.Time

type Options struct {
	ServiceName string
	Version     string
	Now         Clock
}

type Service struct {
	serviceName string
	version     string
	now         Clock
}

type Snapshot struct {
	Status      string
	ServiceName string
	Version     string
	Time        time.Time
}

func New(options Options) *Service {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	return &Service{
		serviceName: options.ServiceName,
		version:     options.Version,
		now:         now,
	}
}

func (s *Service) Check(ctx context.Context) Snapshot {
	_ = ctx
	return Snapshot{
		Status:      StatusOK,
		ServiceName: s.serviceName,
		Version:     s.version,
		Time:        s.now().UTC(),
	}
}
