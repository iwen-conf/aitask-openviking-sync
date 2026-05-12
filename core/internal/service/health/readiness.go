package health

import (
	"context"
)

const (
	ReadyStatusReady    = "ready"
	ReadyStatusDegraded = "degraded"
	ReadyStatusNotReady = "not_ready"

	DependencyStatusOK          = "ok"
	DependencyStatusUnavailable = "unavailable"
)

type CheckFunc func(ctx context.Context) error

type Dependency struct {
	Name     string
	Critical bool
	Check    CheckFunc
}

type ReadinessOptions struct {
	Dependencies []Dependency
}

type ReadinessService struct {
	dependencies []Dependency
}

type ReadinessSnapshot struct {
	Status       string
	Dependencies map[string]string
}

func NewReadiness(options ReadinessOptions) *ReadinessService {
	dependencies := make([]Dependency, 0, len(options.Dependencies))
	for _, dependency := range options.Dependencies {
		if dependency.Name == "" || dependency.Check == nil {
			continue
		}
		dependencies = append(dependencies, dependency)
	}

	return &ReadinessService{
		dependencies: dependencies,
	}
}

func (s *ReadinessService) Check(ctx context.Context) ReadinessSnapshot {
	dependencies := make(map[string]string, len(s.dependencies))
	criticalFailed := false
	degraded := false

	for _, dependency := range s.dependencies {
		err := dependency.Check(ctx)
		if err != nil {
			dependencies[dependency.Name] = DependencyStatusUnavailable
			if dependency.Critical {
				criticalFailed = true
			} else {
				degraded = true
			}
			continue
		}
		dependencies[dependency.Name] = DependencyStatusOK
	}

	status := ReadyStatusReady
	if criticalFailed {
		status = ReadyStatusNotReady
	} else if degraded {
		status = ReadyStatusDegraded
	}

	return ReadinessSnapshot{
		Status:       status,
		Dependencies: dependencies,
	}
}
