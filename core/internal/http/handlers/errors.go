package handlers

import "github.com/gin-gonic/gin"

type apiErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retriable bool           `json:"retriable"`
	Details   map[string]any `json:"details"`
}

func writeAPIError(c *gin.Context, statusCode int, code string, message string, retriable bool, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}

	c.AbortWithStatusJSON(statusCode, apiErrorEnvelope{
		Code:      code,
		Message:   message,
		Retriable: retriable,
		Details:   details,
	})
}
