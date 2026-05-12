package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/health"
)

type HealthHandler struct {
	health    *health.Service
	readiness *health.ReadinessService
}

func NewHealthHandler(healthService *health.Service, readinessService *health.ReadinessService) *HealthHandler {
	return &HealthHandler{
		health:    healthService,
		readiness: readinessService,
	}
}

func (h *HealthHandler) Healthz(c *gin.Context) {
	snapshot := h.health.Check(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{
		"status": snapshot.Status,
		"time":   snapshot.Time.Format(health.TimeFormat),
	})
}

func (h *HealthHandler) Readyz(c *gin.Context) {
	if h.readiness == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":       health.ReadyStatusNotReady,
			"dependencies": gin.H{},
		})
		return
	}

	snapshot := h.readiness.Check(c.Request.Context())
	statusCode := http.StatusOK
	if snapshot.Status == health.ReadyStatusNotReady {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, gin.H{
		"status":       snapshot.Status,
		"dependencies": snapshot.Dependencies,
	})
}
