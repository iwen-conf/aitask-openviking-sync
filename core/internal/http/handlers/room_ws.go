package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/agents"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/room"
)

type verifierForWS interface {
	VerifyToken(ctx context.Context, plainToken string) (identity.AgentIdentity, error)
}

type RoomWebSocketHandler struct {
	rooms         *room.Service
	verifier      verifierForWS
	operatorLabel string
	upgrader      websocket.Upgrader
}

func NewRoomWebSocketHandler(rooms *room.Service, verifier verifierForWS, operatorLabel string) *RoomWebSocketHandler {
	return &RoomWebSocketHandler{
		rooms:         rooms,
		verifier:      verifier,
		operatorLabel: strings.TrimSpace(operatorLabel),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

func (h *RoomWebSocketHandler) Connect(c *gin.Context) {
	if h.rooms == nil {
		writeAPIError(c, http.StatusServiceUnavailable, "ROOM_NOT_FOUND", "Room service unavailable", true, map[string]any{})
		return
	}

	projectID := strings.TrimSpace(c.Param("projectId"))
	actor, ok := h.resolveIdentity(c)
	if !ok {
		return
	}
	if actor.IsAgent() && !actor.Agent.CanAccessProject(projectID) {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "No access to this project", false, map[string]any{"projectId": projectID})
		return
	}
	if actor.IsAgent() && !actor.Agent.HasScope("room:connect") && !actor.Agent.HasScope("room:read") {
		writeAPIError(c, http.StatusForbidden, "PROJECT_ACCESS_DENIED", "room:read scope required", false, map[string]any{"projectId": projectID})
		return
	}

	roomSnapshot, err := h.rooms.TouchPresence(c.Request.Context(), projectID, actor, true)
	if err != nil {
		writeAPIError(c, http.StatusNotFound, "ROOM_NOT_FOUND", "Room not found", false, map[string]any{"projectId": projectID})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sender := room.SenderFromIdentity(actor, h.operatorLabel)
	h.rooms.PublishPresence(projectID, roomSnapshot.RoomID, "room.member_online", sender, map[string]any{"projectId": projectID})

	eventCh, cancel := h.rooms.Subscribe(projectID)
	defer cancel()

	connected := room.Envelope{
		EventID:   "evt_connected_" + time.Now().UTC().Format("20060102150405.000000000"),
		EventType: "room.connected",
		ProjectID: projectID,
		RoomID:    roomSnapshot.RoomID,
		Sender:    &sender,
		Payload: map[string]any{
			"projectId": projectID,
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := conn.WriteJSON(connected); err != nil {
		_, _ = h.rooms.TouchPresence(c.Request.Context(), projectID, actor, false)
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			_, _ = h.rooms.TouchPresence(c.Request.Context(), projectID, actor, true)

			var ping struct {
				Type string `json:"type"`
			}
			if jsonErr := json.Unmarshal(payload, &ping); jsonErr == nil {
				if strings.EqualFold(strings.TrimSpace(ping.Type), "ping") {
					continue
				}
			}
		}
	}()

	for {
		select {
		case envelope, ok := <-eventCh:
			if !ok {
				_, _ = h.rooms.TouchPresence(c.Request.Context(), projectID, actor, false)
				h.rooms.PublishPresence(projectID, roomSnapshot.RoomID, "room.member_offline", sender, map[string]any{"projectId": projectID})
				return
			}
			if err := conn.WriteJSON(envelope); err != nil {
				_, _ = h.rooms.TouchPresence(c.Request.Context(), projectID, actor, false)
				h.rooms.PublishPresence(projectID, roomSnapshot.RoomID, "room.member_offline", sender, map[string]any{"projectId": projectID})
				return
			}
		case <-done:
			_, _ = h.rooms.TouchPresence(c.Request.Context(), projectID, actor, false)
			h.rooms.PublishPresence(projectID, roomSnapshot.RoomID, "room.member_offline", sender, map[string]any{"projectId": projectID})
			return
		}
	}
}

func (h *RoomWebSocketHandler) resolveIdentity(c *gin.Context) (identity.Identity, bool) {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		label := strings.TrimSpace(h.operatorLabel)
		if label == "" {
			label = "local-operator"
		}
		return identity.Operator(label), true
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeAPIError(c, http.StatusUnauthorized, "AGENT_TOKEN_INVALID", "Agent token is invalid", false, map[string]any{})
		return identity.Identity{}, false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" || h.verifier == nil {
		writeAPIError(c, http.StatusUnauthorized, "AGENT_TOKEN_INVALID", "Agent token is invalid", false, map[string]any{})
		return identity.Identity{}, false
	}

	agentIdentity, err := h.verifier.VerifyToken(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, agents.ErrAgentTokenExpired) {
			writeAPIError(c, http.StatusUnauthorized, "AGENT_TOKEN_EXPIRED", "Agent token expired", false, map[string]any{})
			return identity.Identity{}, false
		}
		writeAPIError(c, http.StatusUnauthorized, "AGENT_TOKEN_INVALID", "Agent token is invalid", false, map[string]any{})
		return identity.Identity{}, false
	}

	return identity.Identity{SenderType: identity.SenderTypeAgent, Agent: agentIdentity}, true
}
