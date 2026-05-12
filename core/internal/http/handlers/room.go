package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/http/middleware"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/room"
)

type RoomHandler struct {
	service *room.Service
}

func NewRoomHandler(service *room.Service) *RoomHandler {
	return &RoomHandler{service: service}
}

type sendRoomMessageRequest struct {
	MessageType string         `json:"messageType"`
	Content     string         `json:"content"`
	Payload     map[string]any `json:"payload"`
}

type pinMessageRequest struct {
	As string `json:"as"`
}

func (h *RoomHandler) GetRoom(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("room:read") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:read scope required", false, map[string]any{})
		return
	}
	item, err := h.service.GetRoom(c.Request.Context(), projectID)
	if err != nil {
		h.writeRoomError(c, projectID, "", err)
		return
	}
	c.JSON(http.StatusOK, roomToJSON(item))
}

func (h *RoomHandler) ListMessages(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("room:history") && !actor.Agent.HasScope("room:read") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:history scope required", false, map[string]any{})
		return
	}
	limit := parseInt(c.Query("limit"), 50)
	before := strings.TrimSpace(c.Query("before"))
	items, nextCursor, err := h.service.ListMessages(c.Request.Context(), projectID, limit, before)
	if err != nil {
		h.writeRoomError(c, projectID, "", err)
		return
	}
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		result = append(result, roomMessageToJSON(item))
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "nextCursor": nextCursor})
}

func (h *RoomHandler) SendMessage(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("room:write") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:write scope required", false, map[string]any{})
		return
	}
	var req sendRoomMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}

	item, err := h.service.SendMessage(c.Request.Context(), actor, projectID, room.SendMessageInput{
		MessageType: req.MessageType,
		Content:     req.Content,
		Payload:     req.Payload,
	})
	if err != nil {
		h.writeRoomError(c, projectID, "", err)
		return
	}
	c.JSON(http.StatusOK, roomMessageToJSON(item))
}

func (h *RoomHandler) ListMentions(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("room:mention") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:mention scope required", false, map[string]any{})
		return
	}
	items, err := h.service.ListMentions(c.Request.Context(), actor, projectID, false)
	if err != nil {
		h.writeRoomError(c, projectID, "", err)
		return
	}
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		result = append(result, roomMentionToJSON(item))
	}
	c.JSON(http.StatusOK, gin.H{"items": result})
}

func (h *RoomHandler) GetUnreadMentions(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("room:mention") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:mention scope required", false, map[string]any{})
		return
	}
	count, err := h.service.UnreadMentions(c.Request.Context(), actor, projectID)
	if err != nil {
		h.writeRoomError(c, projectID, "", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *RoomHandler) MarkMessageRead(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	messageID := strings.TrimSpace(c.Param("messageId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("room:mention") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:mention scope required", false, map[string]any{})
		return
	}
	updated, err := h.service.MarkMentionsHandledByMessage(c.Request.Context(), actor, projectID, messageID)
	if err != nil {
		h.writeRoomError(c, projectID, messageID, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"messageId": messageID, "handledMentions": updated})
}

func (h *RoomHandler) PinMessage(c *gin.Context) {
	projectID := strings.TrimSpace(c.Param("projectId"))
	messageID := strings.TrimSpace(c.Param("messageId"))
	actor := middleware.IdentityFromContext(c)
	if actor.IsAgent() && !actor.Agent.HasScope("room:pin") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:pin scope required", false, map[string]any{})
		return
	}
	var req pinMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body", false, map[string]any{"error": err.Error()})
		return
	}
	message, err := h.service.PinMessage(c.Request.Context(), actor, projectID, messageID, req.As)
	if err != nil {
		h.writeRoomError(c, projectID, messageID, err)
		return
	}
	c.JSON(http.StatusOK, roomMessageToJSON(message))
}

func (h *RoomHandler) writeRoomError(c *gin.Context, projectID string, messageID string, err error) {
	var invalid *room.InvalidInputError
	if errors.As(err, &invalid) {
		writeAPIError(c, http.StatusBadRequest, "INVALID_ARGUMENT", "Invalid room request", false, stringMapToAny(invalid.Details()))
		return
	}
	if errors.Is(err, room.ErrRoomNotFound) {
		writeAPIError(c, http.StatusNotFound, "ROOM_NOT_FOUND", "Room not found", false, map[string]any{"projectId": projectID, "messageId": messageID})
		return
	}
	if errors.Is(err, room.ErrRoomAccessDenied) {
		writeAPIError(c, http.StatusForbidden, "ROOM_ACCESS_DENIED", "No access to room", false, map[string]any{"projectId": projectID})
		return
	}
	if errors.Is(err, room.ErrProjectArchived) {
		writeAPIError(c, http.StatusConflict, "PROJECT_ACCESS_DENIED", "Project is archived and read-only", false, map[string]any{"projectId": projectID, "status": "archived"})
		return
	}
	if errors.Is(err, room.ErrRoomMessageTooLarge) {
		writeAPIError(c, http.StatusBadRequest, "ROOM_MESSAGE_TOO_LARGE", "Room message exceeds limit", false, map[string]any{})
		return
	}
	writeAPIError(c, http.StatusInternalServerError, "INTERNAL", "Internal server error", true, map[string]any{})
}

func roomToJSON(item room.Room) gin.H {
	members := make([]gin.H, 0, len(item.Members))
	for _, member := range item.Members {
		members = append(members, gin.H{
			"memberType":    member.MemberType,
			"online":        member.Online,
			"agentId":       nullableStringPointer(member.AgentID),
			"agentType":     nullableStringPointer(member.AgentType),
			"operatorLabel": nullableStringPointer(member.OperatorLabel),
		})
	}
	return gin.H{
		"roomId":        item.RoomID,
		"projectId":     item.ProjectID,
		"status":        item.Status,
		"operatorLabel": item.OperatorLabel,
		"members":       members,
	}
}

func roomMessageToJSON(item room.Message) gin.H {
	return gin.H{
		"messageId":   item.MessageID,
		"projectId":   item.ProjectID,
		"roomId":      item.RoomID,
		"sender":      roomSenderToJSON(item.Sender),
		"messageType": item.MessageType,
		"content":     item.Content,
		"payload":     item.Payload,
		"createdAt":   item.CreatedAt,
	}
}

func roomSenderToJSON(sender room.MessageSender) gin.H {
	return gin.H{
		"type":          sender.Type,
		"agentId":       nullableStringPointer(sender.AgentID),
		"agentType":     nullableStringPointer(sender.AgentType),
		"operatorLabel": nullableStringPointer(sender.OperatorLabel),
	}
}

func roomMentionToJSON(item room.Mention) gin.H {
	return gin.H{
		"mentionId":          item.MentionID,
		"messageId":          item.MessageID,
		"mentionedAgentType": nullableStringPointer(item.MentionedAgentType),
		"mentionedAgentId":   nullableStringPointer(item.MentionedAgentID),
		"operatorLabel":      nullableStringPointer(item.MentionedOperator),
		"handled":            item.Handled,
		"handledAt":          nullableTimePointer(item.HandledAt),
		"createdAt":          item.CreatedAt,
	}
}

func parseInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
