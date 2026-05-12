package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	_ "github.com/jackc/pgx/v5/stdlib"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/config"
	httpserver "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/rpc"
	agentsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
	contextsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/context"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/health"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	projectsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/projects"
	roomsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/room"
	tasksvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/tasks"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/worker"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/pkg/ids"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	healthService := health.New(health.Options{ServiceName: cfg.Service.Name, Version: cfg.Service.Version})

	postgresDB, err := sql.Open("pgx", cfg.Postgres.URL)
	if err != nil {
		slog.Error("failed to initialize postgres client", "error", err)
		os.Exit(1)
	}
	defer postgresDB.Close()

	dragonflyOptions, err := redis.ParseURL(cfg.Dragonfly.URL)
	if err != nil {
		slog.Error("failed to parse dragonfly url", "error", err)
		os.Exit(1)
	}
	dragonflyClient := redis.NewClient(dragonflyOptions)
	defer func() {
		if closeErr := dragonflyClient.Close(); closeErr != nil {
			slog.Warn("failed to close dragonfly client", "error", closeErr)
		}
	}()

	readinessService := health.NewReadiness(health.ReadinessOptions{
		Dependencies: []health.Dependency{
			{Name: "postgres", Critical: true, Check: health.SQLPingChecker(postgresDB)},
			{Name: "dragonfly", Critical: true, Check: health.RedisPingChecker(func(ctx context.Context) error {
				return dragonflyClient.Ping(ctx).Err()
			})},
		},
	})

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 3*time.Second)
	startupSnapshot := readinessService.Check(startupCtx)
	startupCancel()
	if startupSnapshot.Status == health.ReadyStatusNotReady {
		slog.Error("critical dependency check failed on startup", "dependencies", startupSnapshot.Dependencies)
		os.Exit(1)
	}
	if startupSnapshot.Status == health.ReadyStatusDegraded {
		slog.Warn("service startup in degraded state", "dependencies", startupSnapshot.Dependencies)
	}

	agentService, err := agentsvc.New(agentsvc.Options{DB: postgresDB, TokenSecret: cfg.Security.AgentTokenSecret})
	if err != nil {
		slog.Error("failed to initialize agent service", "error", err)
		os.Exit(1)
	}

	openVikingSettingsStore, err := openviking.NewSettingsStore(postgresDB, cfg.OpenViking.SettingsKey)
	if err != nil {
		slog.Error("failed to initialize openviking settings store", "error", err)
		os.Exit(1)
	}
	openVikingProjectFactory := openviking.NewProjectClientFactory(openVikingSettingsStore, &stdhttp.Client{Timeout: 8 * time.Second}, postgresDB, cfg.OpenViking.Namespace)
	openVikingWriter := openviking.NewProjectAwareWriter(openVikingProjectFactory, slog.Default())

	roomService, err := roomsvc.New(roomsvc.Options{
		DB:                   postgresDB,
		ConsoleOperatorLabel: cfg.Console.OperatorLabel,
		MemoryWriter:         openVikingWriter,
		PresenceStore:        dragonflyClient,
	})
	if err != nil {
		slog.Error("failed to initialize room service", "error", err)
		os.Exit(1)
	}

	contextService, err := contextsvc.New(contextsvc.Options{
		DB:                  postgresDB,
		MemoryWriter:        openVikingWriter,
		HandoffPublisher:    roomService,
		OpenVikingNamespace: cfg.OpenViking.Namespace,
	})
	if err != nil {
		slog.Error("failed to initialize context service", "error", err)
		os.Exit(1)
	}

	projectService, err := projectsvc.New(projectsvc.Options{
		DB:                   postgresDB,
		ConsoleOperatorLabel: cfg.Console.OperatorLabel,
		OpenVikingNamespace:  cfg.OpenViking.Namespace,
		Hooks: projectsvc.CreateProjectHooks{
			InitializeOpenViking: func(ctx context.Context, _ *sql.Tx, payload projectsvc.HookPayload) error {
				return openviking.SeedProjectSpace(ctx, openVikingWriter, payload.ProjectID, openviking.ProjectSeedInput{
					ProjectName:        payload.Name,
					ProjectGoal:        payload.Goal,
					ProjectDescription: payload.Description,
				})
			},
			InitializeProjectRoom: func(ctx context.Context, tx *sql.Tx, payload projectsvc.HookPayload) (string, error) {
				return roomService.EnsureProjectRoomTx(ctx, tx, payload.ProjectID, payload.RoomID)
			},
			BindDefaultAgents: func(ctx context.Context, tx *sql.Tx, payload projectsvc.HookPayload) error {
				defaultTypes := []string{"claude-code", "codex", "gemini"}
				for _, agentType := range defaultTypes {
					template, ok := agentsvc.DefaultTemplateByType(agentType)
					if !ok {
						continue
					}
					agentID := ""
					var status string
					queryErr := tx.QueryRowContext(ctx, `
						SELECT id, status
						FROM agents
						WHERE agent_type = $1
						ORDER BY created_at ASC
						LIMIT 1
					`, agentType).Scan(&agentID, &status)
					if errors.Is(queryErr, sql.ErrNoRows) {
						agentID = ids.New(ids.PrefixAgent)
						if _, err := tx.ExecContext(ctx, `
							INSERT INTO agents (id, name, agent_type, role, default_model, status)
							VALUES ($1, $2, $3, $4, $5, 'active')
						`, agentID, "default-"+agentType, template.AgentType, template.Role, template.DefaultModel); err != nil {
							return err
						}
						for _, skill := range template.Skills {
							if _, err := tx.ExecContext(ctx, `
								INSERT INTO agent_skills (agent_id, skill_name)
								VALUES ($1, $2)
								ON CONFLICT (agent_id, skill_name) DO NOTHING
							`, agentID, skill); err != nil {
								return err
							}
						}
						for _, model := range template.Models {
							if _, err := tx.ExecContext(ctx, `
								INSERT INTO agent_models (agent_id, model_name)
								VALUES ($1, $2)
								ON CONFLICT (agent_id, model_name) DO NOTHING
							`, agentID, model); err != nil {
								return err
							}
						}
					} else if queryErr != nil {
						return queryErr
					} else {
						if strings.TrimSpace(status) != "active" {
							if _, err := tx.ExecContext(ctx, `UPDATE agents SET status = 'active' WHERE id = $1`, agentID); err != nil {
								return err
							}
						}
						if _, err := tx.ExecContext(ctx, `
							UPDATE agents
							SET role = $2,
								default_model = $3
							WHERE id = $1
						`, agentID, template.Role, template.DefaultModel); err != nil {
							return err
						}
						for _, skill := range template.Skills {
							if _, err := tx.ExecContext(ctx, `
								INSERT INTO agent_skills (agent_id, skill_name)
								VALUES ($1, $2)
								ON CONFLICT (agent_id, skill_name) DO NOTHING
							`, agentID, skill); err != nil {
								return err
							}
						}
						for _, model := range template.Models {
							if _, err := tx.ExecContext(ctx, `
								INSERT INTO agent_models (agent_id, model_name)
								VALUES ($1, $2)
								ON CONFLICT (agent_id, model_name) DO NOTHING
							`, agentID, model); err != nil {
								return err
							}
						}
					}

					if _, err := tx.ExecContext(ctx, `
						INSERT INTO agent_project_bindings (agent_id, project_id, role, enabled)
						VALUES ($1, $2, $3, TRUE)
						ON CONFLICT (agent_id, project_id)
						DO UPDATE SET role = EXCLUDED.role, enabled = TRUE
					`, agentID, payload.ProjectID, template.Role); err != nil {
						return err
					}
				}
				return nil
			},
		},
		OnProjectCompleted: roomService.PublishProjectCompleted,
	})
	if err != nil {
		slog.Error("failed to initialize project service", "error", err)
		os.Exit(1)
	}

	tasksService, err := tasksvc.New(tasksvc.Options{
		DB:                   postgresDB,
		ConsoleOperatorLabel: cfg.Console.OperatorLabel,
		MemoryWriter:         openVikingWriter,
		EventSink:            roomService,
	})
	if err != nil {
		slog.Error("failed to initialize task service", "error", err)
		os.Exit(1)
	}

	router := httpserver.NewRouter(httpserver.RouterDeps{
		DB:                   postgresDB,
		Health:               healthService,
		Readiness:            readinessService,
		Projects:             projectService,
		Agents:               agentService,
		Tasks:                tasksService,
		OpenViking:           openVikingWriter,
		OpenVikingResources:  openVikingWriter,
		OpenVikingSettings:   openVikingSettingsStore,
		Room:                 roomService,
		Context:              contextService,
		ConsoleOperatorLabel: cfg.Console.OperatorLabel,
		OpenVikingNamespace:  cfg.OpenViking.Namespace,
		RateLimit: middleware.RateLimitOptions{
			Enabled:         cfg.RateLimit.Enabled,
			Store:           dragonflyClient,
			Capacity:        cfg.RateLimit.Capacity,
			RefillPerSecond: cfg.RateLimit.RefillPerSecond,
			KeyPrefix:       cfg.RateLimit.KeyPrefix,
		},
	})

	rpcHandler := rpc.NewHandler(rpc.ServerDeps{
		Agents:               agentService,
		Tasks:                tasksService,
		Projects:             projectService,
		Context:              contextService,
		Room:                 roomService,
		OpenViking:           openVikingWriter,
		ConsoleOperatorLabel: cfg.Console.OperatorLabel,
		RateLimit: rpc.RateLimitOptions{
			Enabled:         cfg.RateLimit.Enabled,
			Store:           dragonflyClient,
			Capacity:        cfg.RateLimit.Capacity,
			RefillPerSecond: cfg.RateLimit.RefillPerSecond,
			KeyPrefix:       cfg.RateLimit.KeyPrefix,
		},
		Logger: slog.Default(),
	})

	rootMux := stdhttp.NewServeMux()
	rootMux.Handle("/aitask.v1.AgentService/", rpcHandler)
	rootMux.Handle("/aitask.v1.BootstrapService/", rpcHandler)
	rootMux.Handle("/aitask.v1.ContextService/", rpcHandler)
	rootMux.Handle("/aitask.v1.TaskService/", rpcHandler)
	rootMux.Handle("/", router)

	server := httpserver.NewServer(cfg.Server, rootMux)

	var scheduler *worker.Scheduler
	if cfg.Worker.Enabled {
		scheduler = worker.NewScheduler(worker.SchedulerOptions{Logger: slog.Default()})

		_, _ = scheduler.ScheduleDelayed(cfg.Worker.StartDelay, "worker.bootstrap.maintenance", func(ctx context.Context) error {
			_, _ = tasksService.BlockTimedOutRunningTasks(ctx, cfg.Worker.ActiveRunTimeout, cfg.Worker.BatchSize)
			_, _ = tasksService.EnsureSubmittedReviewTasks(ctx, cfg.Worker.BatchSize)
			_, _ = projectService.RefreshProgress(ctx, cfg.Worker.BatchSize)
			_, _ = roomService.CleanupStalePresence(ctx, cfg.Worker.PresenceTTL, cfg.Worker.BatchSize)
			_, _ = roomService.GenerateDailySummaries(ctx, time.Now().UTC(), cfg.Worker.BatchSize)
			_, _ = tasksService.SyncTaskSummaries(ctx, cfg.Worker.BatchSize)
			_, _ = contextService.SyncPendingHandoffs(ctx, cfg.Worker.BatchSize)
			return nil
		})

		if err := scheduler.ScheduleEvery(cfg.Worker.ActiveRunSweepInterval, "worker.active_run_timeout", func(ctx context.Context) error {
			_, err := tasksService.BlockTimedOutRunningTasks(ctx, cfg.Worker.ActiveRunTimeout, cfg.Worker.BatchSize)
			return err
		}); err != nil {
			slog.Error("failed to schedule active run timeout job", "error", err)
			os.Exit(1)
		}
		if err := scheduler.ScheduleEvery(cfg.Worker.ReviewSweepInterval, "worker.submitted_review_task", func(ctx context.Context) error {
			_, err := tasksService.EnsureSubmittedReviewTasks(ctx, cfg.Worker.BatchSize)
			return err
		}); err != nil {
			slog.Error("failed to schedule review fallback job", "error", err)
			os.Exit(1)
		}
		if err := scheduler.ScheduleEvery(cfg.Worker.ProgressSweepInterval, "worker.project_progress", func(ctx context.Context) error {
			_, err := projectService.RefreshProgress(ctx, cfg.Worker.BatchSize)
			return err
		}); err != nil {
			slog.Error("failed to schedule progress job", "error", err)
			os.Exit(1)
		}
		if err := scheduler.ScheduleEvery(cfg.Worker.PresenceSweepInterval, "worker.presence_cleanup", func(ctx context.Context) error {
			_, err := roomService.CleanupStalePresence(ctx, cfg.Worker.PresenceTTL, cfg.Worker.BatchSize)
			return err
		}); err != nil {
			slog.Error("failed to schedule presence cleanup job", "error", err)
			os.Exit(1)
		}
		if err := scheduler.ScheduleEvery(cfg.Worker.TaskSummarySweepInterval, "worker.task_summary_sync", func(ctx context.Context) error {
			_, err := tasksService.SyncTaskSummaries(ctx, cfg.Worker.BatchSize)
			return err
		}); err != nil {
			slog.Error("failed to schedule task summary sync job", "error", err)
			os.Exit(1)
		}
		if err := scheduler.ScheduleEvery(cfg.Worker.HandoffSweepInterval, "worker.handoff_sync", func(ctx context.Context) error {
			_, err := contextService.SyncPendingHandoffs(ctx, cfg.Worker.BatchSize)
			return err
		}); err != nil {
			slog.Error("failed to schedule handoff sync job", "error", err)
			os.Exit(1)
		}

		var completionNoticeMu sync.Mutex
		completionNoticeAt := map[string]time.Time{}
		if err := scheduler.ScheduleEvery(cfg.Worker.CompletionSweepInterval, "worker.completion_policy_check", func(ctx context.Context) error {
			candidates, err := projectService.FindCompletableProjects(ctx, cfg.Worker.BatchSize)
			if err != nil {
				return err
			}
			for _, candidate := range candidates {
				completionNoticeMu.Lock()
				lastAt := completionNoticeAt[candidate.ProjectID]
				if !lastAt.IsZero() && time.Since(lastAt) < 30*time.Minute {
					completionNoticeMu.Unlock()
					continue
				}
				completionNoticeAt[candidate.ProjectID] = time.Now().UTC()
				completionNoticeMu.Unlock()

				_, _ = roomService.SendSystemMessage(ctx, candidate.ProjectID, "review_request", "Project completion policy satisfied, reviewer can confirm completion.", map[string]any{
					"projectId": candidate.ProjectID,
					"eventType": "project.ready_for_review",
				})
			}
			return nil
		}); err != nil {
			slog.Error("failed to schedule completion policy check job", "error", err)
			os.Exit(1)
		}

		if err := scheduler.ScheduleCron(cfg.Worker.DailySummaryCron, "worker.room_daily_summary", func(ctx context.Context) error {
			_, err := roomService.GenerateDailySummaries(ctx, time.Now().UTC(), cfg.Worker.BatchSize)
			return err
		}); err != nil {
			slog.Error("failed to schedule daily summary job", "error", err)
			os.Exit(1)
		}

		scheduler.Start()
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("starting HTTP server", "addr", cfg.Server.Addr(), "env", cfg.Environment)
		errCh <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		if scheduler != nil {
			shutdownWorkerCtx, shutdownWorkerCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := scheduler.Shutdown(shutdownWorkerCtx); err != nil {
				slog.Warn("worker scheduler shutdown failed", "error", err)
			}
			shutdownWorkerCancel()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to gracefully shutdown HTTP server", "error", err)
			os.Exit(1)
		}
		slog.Info("HTTP server stopped")
	case err := <-errCh:
		if scheduler != nil {
			shutdownWorkerCtx, shutdownWorkerCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if shutdownErr := scheduler.Shutdown(shutdownWorkerCtx); shutdownErr != nil {
				slog.Warn("worker scheduler shutdown failed", "error", shutdownErr)
			}
			shutdownWorkerCancel()
		}
		if err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}
}
