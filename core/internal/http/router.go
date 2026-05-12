package httpserver

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/handlers"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
	contextsvc "gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/context"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/health"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/room"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/tasks"
)

type RouterDeps struct {
	DB                   *sql.DB
	Health               *health.Service
	Readiness            *health.ReadinessService
	Projects             handlers.ProjectService
	Agents               *agents.Service
	Tasks                *tasks.Service
	OpenViking           openviking.MemoryClient
	OpenVikingResources  handlers.OpenVikingGitResourceRegistrar
	OpenVikingSettings   handlers.OpenVikingSettingsStore
	Room                 *room.Service
	Context              *contextsvc.Service
	ConsoleOperatorLabel string
	OpenVikingNamespace  string
	RateLimit            middleware.RateLimitOptions
}

func NewRouter(deps RouterDeps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	healthHandler := handlers.NewHealthHandler(deps.Health, deps.Readiness)
	r.GET("/healthz", healthHandler.Healthz)
	r.GET("/readyz", healthHandler.Readyz)
	if deps.Room != nil {
		wsHandler := handlers.NewRoomWebSocketHandler(deps.Room, deps.Agents, deps.ConsoleOperatorLabel)
		r.GET("/ws/projects/:projectId/agent-room", wsHandler.Connect)
	}

	api := r.Group("/api")
	api.Use(middleware.RateLimitTokenBucket(deps.RateLimit))
	api.Use(middleware.ResolveIdentity(middleware.ResolverOptions{
		ConsoleOperatorLabel: deps.ConsoleOperatorLabel,
		Verifier:             deps.Agents,
	}))

	if deps.Projects != nil {
		projectsHandler := handlers.NewProjectsHandler(deps.Projects)
		api.GET("/projects", projectsHandler.ListProjects)
		api.POST("/projects", projectsHandler.CreateProject)
		api.GET("/projects/:projectId", projectsHandler.GetProject)
		api.PATCH("/projects/:projectId", projectsHandler.UpdateProject)
		api.POST("/projects/:projectId/complete", projectsHandler.CompleteProject)
		api.POST("/projects/:projectId/archive", projectsHandler.ArchiveProject)
	}

	if deps.Agents != nil {
		agentsHandler := handlers.NewAgentsHandler(deps.Agents)
		api.GET("/agents", agentsHandler.ListAgents)
		api.POST("/agents", agentsHandler.CreateAgent)
		api.POST("/agents/:agentId/tokens", agentsHandler.IssueToken)
		api.POST("/agents/:agentId/tokens/:tokenId/revoke", agentsHandler.RevokeToken)
		api.POST("/projects/:projectId/agents/:agentId/bind", agentsHandler.BindProject)
	}

	projectScoped := api.Group("/projects/:projectId")
	projectScoped.Use(middleware.RequireProjectAccess())

	if deps.Tasks != nil {
		tasksHandler := handlers.NewTasksHandler(deps.Tasks)
		projectScoped.GET("/tasks", tasksHandler.ListTasks)
		projectScoped.POST("/tasks", tasksHandler.CreateTask)
		projectScoped.GET("/tasks/:taskId", tasksHandler.GetTask)
		projectScoped.GET("/tasks/:taskId/events", tasksHandler.ListTaskEvents)
		projectScoped.PATCH("/tasks/:taskId", tasksHandler.UpdateTask)
		projectScoped.POST("/tasks/:taskId/delegate", tasksHandler.DelegateTask)
		projectScoped.POST("/tasks/:taskId/cancel", tasksHandler.CancelTask)

		projectScoped.POST("/tasks/:taskId/start", tasksHandler.StartTask)
		projectScoped.POST("/tasks/:taskId/heartbeat", tasksHandler.HeartbeatTask)
		projectScoped.POST("/tasks/:taskId/submit", tasksHandler.SubmitTask)
		projectScoped.POST("/tasks/:taskId/review", tasksHandler.ReviewTask)
		projectScoped.POST("/tasks/:taskId/fail", tasksHandler.FailTask)
		projectScoped.POST("/tasks/:taskId/resume", tasksHandler.ResumeTask)

		artifactsHandler := handlers.NewArtifactsHandler(deps.Tasks)
		projectScoped.GET("/artifacts", artifactsHandler.ListArtifacts)
		projectScoped.GET("/artifacts/:artifactId", artifactsHandler.GetArtifact)
	}

	if deps.Room != nil {
		roomHandler := handlers.NewRoomHandler(deps.Room)
		projectScoped.GET("/room", roomHandler.GetRoom)
		projectScoped.GET("/room/messages", roomHandler.ListMessages)
		projectScoped.POST("/room/messages", roomHandler.SendMessage)
		projectScoped.POST("/room/messages/:messageId/read", roomHandler.MarkMessageRead)
		projectScoped.POST("/room/messages/:messageId/pin", roomHandler.PinMessage)
		projectScoped.GET("/room/mentions", roomHandler.ListMentions)
		projectScoped.GET("/room/mentions/unread", roomHandler.GetUnreadMentions)
	}

	if deps.OpenViking != nil {
		memoryHandler := handlers.NewMemoryHandler(deps.OpenViking, deps.DB, deps.OpenVikingNamespace)
		projectScoped.GET("/memory", memoryHandler.ListMemory)
		projectScoped.GET("/memory/search", memoryHandler.SearchMemory)
		projectScoped.GET("/memory/read", memoryHandler.ReadMemory)
		projectScoped.POST("/memory/write", memoryHandler.WriteMemory)
		projectScoped.GET("/skills", memoryHandler.ListSkills)
		projectScoped.GET("/skills/:skillName", memoryHandler.ShowSkill)
	}

	if deps.OpenVikingResources != nil {
		resourcesHandler := handlers.NewOpenVikingResourcesHandler(deps.OpenVikingResources)
		projectScoped.POST("/openviking/resources/git", resourcesHandler.RegisterGitResource)
		projectScoped.GET("/openviking/resources/git/status", resourcesHandler.GitResourceStatus)
	}

	if deps.OpenVikingSettings != nil {
		openVikingSettingsHandler := handlers.NewOpenVikingSettingsHandler(deps.OpenVikingSettings, nil)
		api.GET("/system/openviking/settings", openVikingSettingsHandler.GetSystemSettings)
		api.PUT("/system/openviking/settings", openVikingSettingsHandler.UpdateSystemSettings)
		api.GET("/system/openviking/status", openVikingSettingsHandler.GetSystemStatus)
	}

	if deps.Context != nil {
		contextHandler := handlers.NewContextHandler(deps.Context, deps.Projects, deps.Room, deps.OpenViking)
		projectScoped.POST("/context/report", contextHandler.Report)
		projectScoped.POST("/context/handoff", contextHandler.CreateHandoff)
		projectScoped.GET("/context/handoff/current", contextHandler.GetCurrentHandoff)
		projectScoped.GET("/bootstrap", contextHandler.Bootstrap)
	}

	return r
}
