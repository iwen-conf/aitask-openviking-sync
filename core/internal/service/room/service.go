package room

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/identity"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/internal/service/openviking"
	"gitea.ezer.heiyu.space/iwen-conf/aitask/core/pkg/ids"
)

var (
	ErrRoomNotFound        = errors.New("room not found")
	ErrRoomAccessDenied    = errors.New("room access denied")
	ErrRoomMessageTooLarge = errors.New("room message too large")
	ErrRoomMessageInvalid  = errors.New("room message invalid")
	ErrProjectArchived     = errors.New("project archived")
)

var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9_-]{2,80})`)

var allowedMessageTypes = map[string]struct{}{
	"text":               {},
	"task_reference":     {},
	"task_status":        {},
	"question":           {},
	"answer":             {},
	"blocker":            {},
	"review_request":     {},
	"review_result":      {},
	"artifact_reference": {},
	"system_event":       {},
	"memory_note":        {},
	"context_handoff":    {},
	"command_request":    {},
}

type MemoryWriter interface {
	Write(ctx context.Context, projectID string, input openviking.WriteInput) (openviking.WriteResult, error)
}

type Options struct {
	DB                   *sql.DB
	ConsoleOperatorLabel string
	MemoryWriter         MemoryWriter
	PresenceStore        redis.Cmdable
	Logger               *slog.Logger
	Now                  func() time.Time
}

type Service struct {
	db              *sql.DB
	operatorLabel   string
	memoryWriter    MemoryWriter
	logger          *slog.Logger
	now             func() time.Time
	hub             *hub
	onlineMu        sync.RWMutex
	onlineMembers   map[string]map[string]time.Time
	presenceStore   redis.Cmdable
	summaryMu       sync.Mutex
	lastSummaryByID map[string]time.Time
}

type Room struct {
	RoomID        string
	ProjectID     string
	Status        string
	OperatorLabel string
	Members       []RoomMember
}

type RoomMember struct {
	MemberType    string
	Online        bool
	AgentID       *string
	AgentType     *string
	OperatorLabel *string
}

type MessageSender struct {
	Type          string
	AgentID       *string
	AgentType     *string
	OperatorLabel *string
}

type Message struct {
	MessageID   string
	ProjectID   string
	RoomID      string
	Sender      MessageSender
	MessageType string
	Content     string
	Payload     map[string]any
	CreatedAt   time.Time
}

type Mention struct {
	MentionID          string
	MessageID          string
	MentionedAgentType *string
	MentionedAgentID   *string
	MentionedOperator  *string
	Handled            bool
	HandledAt          *time.Time
	CreatedAt          time.Time
}

type SendMessageInput struct {
	MessageType string
	Content     string
	Payload     map[string]any
}

type Envelope struct {
	EventID   string         `json:"eventId"`
	EventType string         `json:"eventType"`
	ProjectID string         `json:"projectId"`
	RoomID    string         `json:"roomId"`
	Sender    *MessageSender `json:"sender,omitempty"`
	Payload   any            `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
}

type InvalidInputError struct {
	details map[string]string
}

func (e *InvalidInputError) Error() string {
	return "invalid room input"
}

func (e *InvalidInputError) Details() map[string]string {
	if len(e.details) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(e.details))
	for k, v := range e.details {
		out[k] = v
	}
	return out
}

func newInvalidInputError(details map[string]string) error {
	copied := make(map[string]string, len(details))
	for k, v := range details {
		copied[k] = v
	}
	return &InvalidInputError{details: copied}
}

func New(opts Options) (*Service, error) {
	if opts.DB == nil {
		return nil, errors.New("room service requires database handle")
	}
	operatorLabel := strings.TrimSpace(opts.ConsoleOperatorLabel)
	if operatorLabel == "" {
		operatorLabel = "local-operator"
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		db:              opts.DB,
		operatorLabel:   operatorLabel,
		memoryWriter:    opts.MemoryWriter,
		presenceStore:   opts.PresenceStore,
		logger:          logger,
		now:             now,
		hub:             newHub(),
		onlineMembers:   map[string]map[string]time.Time{},
		lastSummaryByID: map[string]time.Time{},
	}, nil
}

func (s *Service) EnsureProjectRoomTx(ctx context.Context, tx *sql.Tx, projectID string, preferredRoomID string) (string, error) {
	if tx == nil {
		return "", errors.New("room init requires transaction")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	var roomID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM project_rooms WHERE project_id = $1`, projectID).Scan(&roomID)
	if err == nil {
		if err := s.ensureOperatorMemberTx(ctx, tx, roomID, projectID); err != nil {
			return "", err
		}
		return roomID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("query room failed: %w", err)
	}

	roomID = strings.TrimSpace(preferredRoomID)
	if roomID == "" {
		roomID = ids.New(ids.PrefixRoom)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO project_rooms (id, project_id, status, room_type)
		VALUES ($1, $2, 'active', 'agent_room')
		ON CONFLICT (project_id) DO NOTHING
	`, roomID, projectID); err != nil {
		return "", fmt.Errorf("insert project room failed: %w", err)
	}

	if err := tx.QueryRowContext(ctx, `SELECT id FROM project_rooms WHERE project_id = $1`, projectID).Scan(&roomID); err != nil {
		return "", fmt.Errorf("reload room id failed: %w", err)
	}
	if err := s.ensureOperatorMemberTx(ctx, tx, roomID, projectID); err != nil {
		return "", err
	}
	return roomID, nil
}

func (s *Service) ensureOperatorMemberTx(ctx context.Context, tx *sql.Tx, roomID string, projectID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO project_room_members (
			id, room_id, project_id, member_type, operator_label, role, last_seen_at
		) VALUES ($1,$2,$3,'operator',$4,'owner',NOW())
		ON CONFLICT (room_id, operator_label)
		DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at
	`, ids.New("member"), roomID, projectID, s.operatorLabel)
	if err != nil {
		return fmt.Errorf("ensure operator member failed: %w", err)
	}
	return nil
}

func (s *Service) GetRoom(ctx context.Context, projectID string) (Room, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Room{}, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	var room Room
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, status
		FROM project_rooms
		WHERE project_id = $1
	`, projectID).Scan(&room.RoomID, &room.ProjectID, &room.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Room{}, ErrRoomNotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("get room failed: %w", err)
	}
	room.OperatorLabel = s.operatorLabel

	rows, err := s.db.QueryContext(ctx, `
		SELECT member_type, operator_label, agent_id, agent_type
		FROM project_room_members
		WHERE room_id = $1
		ORDER BY joined_at ASC
	`, room.RoomID)
	if err != nil {
		return Room{}, fmt.Errorf("list room members failed: %w", err)
	}
	defer rows.Close()

	members := make([]RoomMember, 0)
	for rows.Next() {
		var member RoomMember
		var operator sql.NullString
		var agentID sql.NullString
		var agentType sql.NullString
		if err := rows.Scan(&member.MemberType, &operator, &agentID, &agentType); err != nil {
			return Room{}, fmt.Errorf("scan room member failed: %w", err)
		}
		if operator.Valid {
			value := strings.TrimSpace(operator.String)
			if value != "" {
				member.OperatorLabel = &value
			}
		}
		if agentID.Valid {
			value := strings.TrimSpace(agentID.String)
			if value != "" {
				member.AgentID = &value
			}
		}
		if agentType.Valid {
			value := strings.TrimSpace(agentType.String)
			if value != "" {
				member.AgentType = &value
			}
		}
		member.Online = s.isOnline(projectID, memberKey(member.MemberType, member.AgentID, member.OperatorLabel))
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return Room{}, fmt.Errorf("iterate room members failed: %w", err)
	}
	if len(members) == 0 {
		members = []RoomMember{}
	}
	room.Members = members
	return room, nil
}

func (s *Service) ListMessages(ctx context.Context, projectID string, limit int, beforeMessageID string) ([]Message, *string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, nil, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := `
		SELECT id, room_id, project_id,
			sender_type, sender_operator_label, sender_agent_id, sender_agent_type,
			message_type, COALESCE(content, ''), COALESCE(payload, '{}'::jsonb), created_at
		FROM project_room_messages
		WHERE project_id = $1
	`
	args := []any{projectID}
	if before := strings.TrimSpace(beforeMessageID); before != "" {
		var beforeAt time.Time
		err := s.db.QueryRowContext(ctx, `SELECT created_at FROM project_room_messages WHERE project_id = $1 AND id = $2`, projectID, before).Scan(&beforeAt)
		if err == nil {
			query += ` AND (created_at < $2 OR (created_at = $2 AND id < $3))`
			args = append(args, beforeAt, before)
		}
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("list room messages failed: %w", err)
	}
	defer rows.Close()

	items := make([]Message, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, message)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate room messages failed: %w", err)
	}

	nextCursor := (*string)(nil)
	if len(items) == limit {
		value := items[len(items)-1].MessageID
		nextCursor = &value
	}
	return items, nextCursor, nil
}

func (s *Service) SendMessage(ctx context.Context, actor identity.Identity, projectID string, input SendMessageInput) (Message, error) {
	if !actor.IsAgent() && !actor.IsOperator() && !actor.IsSystem() {
		return Message{}, ErrRoomAccessDenied
	}
	projectID = strings.TrimSpace(projectID)
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return Message{}, err
	}
	input.MessageType = normalizeMessageType(input.MessageType)
	input.Content = strings.TrimSpace(input.Content)

	details := map[string]string{}
	if projectID == "" {
		details["projectId"] = "cannot be empty"
	}
	if input.MessageType == "" {
		details["messageType"] = "cannot be empty"
	}
	if _, ok := allowedMessageTypes[input.MessageType]; !ok {
		details["messageType"] = "is not allowed"
	}
	if len(input.Content) == 0 {
		details["content"] = "cannot be empty"
	}
	if len(input.Content) > 4000 {
		return Message{}, ErrRoomMessageTooLarge
	}
	if len(details) > 0 {
		return Message{}, newInvalidInputError(details)
	}

	room, err := s.GetRoom(ctx, projectID)
	if err != nil {
		return Message{}, err
	}
	if actor.IsAgent() || actor.IsOperator() {
		if err := s.touchPresence(ctx, room.RoomID, projectID, actor, true); err != nil {
			return Message{}, err
		}
	}

	messageID := ids.New(ids.PrefixMessage)
	payload := input.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	rawPayload, _ := json.Marshal(payload)

	senderType, senderOperator, senderAgentID, senderAgentType := senderColumns(actor, s.operatorLabel)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO project_room_messages (
			id, room_id, project_id,
			sender_type, sender_operator_label, sender_agent_id, sender_agent_type,
			message_type, content, payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, messageID, room.RoomID, projectID, senderType, senderOperator, senderAgentID, senderAgentType, input.MessageType, nullableString(input.Content), rawPayload); err != nil {
		return Message{}, fmt.Errorf("insert room message failed: %w", err)
	}

	if err := s.insertMentions(ctx, room.RoomID, messageID, input.Content); err != nil {
		s.logger.Warn("room mention insertion failed", "projectId", projectID, "messageId", messageID, "error", err)
	}

	message, err := s.GetMessage(ctx, projectID, messageID)
	if err != nil {
		return Message{}, err
	}

	s.publish(projectID, s.newEnvelope(projectID, "room.message", room.RoomID, &message.Sender, map[string]any{
		"messageId":   message.MessageID,
		"messageType": message.MessageType,
		"content":     message.Content,
		"payload":     message.Payload,
		"createdAt":   message.CreatedAt,
	}))

	s.maybeWriteRoomSummary(ctx, message)

	return message, nil
}

func (s *Service) SendSystemMessage(ctx context.Context, projectID string, messageType string, content string, payload map[string]any) (Message, error) {
	actor := identity.System()
	message, err := s.SendMessage(ctx, actor, projectID, SendMessageInput{
		MessageType: messageType,
		Content:     content,
		Payload:     payload,
	})
	if err != nil {
		return Message{}, err
	}
	return message, nil
}

func (s *Service) GetMessage(ctx context.Context, projectID string, messageID string) (Message, error) {
	projectID = strings.TrimSpace(projectID)
	messageID = strings.TrimSpace(messageID)
	if projectID == "" || messageID == "" {
		return Message{}, newInvalidInputError(map[string]string{"projectId": "cannot be empty", "messageId": "cannot be empty"})
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, room_id, project_id,
			sender_type, sender_operator_label, sender_agent_id, sender_agent_type,
			message_type, COALESCE(content, ''), COALESCE(payload, '{}'::jsonb), created_at
		FROM project_room_messages
		WHERE project_id = $1 AND id = $2
	`, projectID, messageID)
	message, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Message{}, ErrRoomNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("get room message failed: %w", err)
	}
	return message, nil
}

func (s *Service) ListMentions(ctx context.Context, actor identity.Identity, projectID string, onlyUnhandled bool) ([]Mention, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}

	filters, args := mentionFilter(actor)
	if filters == "" {
		return []Mention{}, nil
	}
	query := `
		SELECT m.id, m.message_id, m.mentioned_agent_type, m.mentioned_agent_id, m.mentioned_operator_label,
			m.handled, m.handled_at, m.created_at
		FROM project_room_mentions m
		JOIN project_room_messages msg ON msg.id = m.message_id
		WHERE msg.project_id = $1 AND (` + filters + `)
	`
	values := []any{projectID}
	values = append(values, args...)
	if onlyUnhandled {
		query += " AND m.handled = FALSE"
	}
	query += " ORDER BY m.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, values...)
	if err != nil {
		return nil, fmt.Errorf("list mentions failed: %w", err)
	}
	defer rows.Close()

	items := make([]Mention, 0)
	for rows.Next() {
		var item Mention
		var mentionedType sql.NullString
		var mentionedID sql.NullString
		var mentionedOperator sql.NullString
		var handledAt sql.NullTime
		if err := rows.Scan(&item.MentionID, &item.MessageID, &mentionedType, &mentionedID, &mentionedOperator, &item.Handled, &handledAt, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mention failed: %w", err)
		}
		if mentionedType.Valid {
			value := mentionedType.String
			item.MentionedAgentType = &value
		}
		if mentionedID.Valid {
			value := mentionedID.String
			item.MentionedAgentID = &value
		}
		if mentionedOperator.Valid {
			value := mentionedOperator.String
			item.MentionedOperator = &value
		}
		if handledAt.Valid {
			value := handledAt.Time
			item.HandledAt = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mentions failed: %w", err)
	}
	if len(items) == 0 {
		return []Mention{}, nil
	}
	return items, nil
}

func (s *Service) UnreadMentions(ctx context.Context, actor identity.Identity, projectID string) (int, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return 0, newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	filters, args := mentionFilter(actor)
	if filters == "" {
		return 0, nil
	}
	query := `
		SELECT COUNT(1)
		FROM project_room_mentions m
		JOIN project_room_messages msg ON msg.id = m.message_id
		WHERE msg.project_id = $1
		  AND m.handled = FALSE
		  AND (` + filters + `)
	`
	values := []any{projectID}
	values = append(values, args...)

	var count int
	if err := s.db.QueryRowContext(ctx, query, values...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count unread mentions failed: %w", err)
	}
	return count, nil
}

func (s *Service) MarkMentionsHandledByMessage(ctx context.Context, actor identity.Identity, projectID string, messageID string) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	messageID = strings.TrimSpace(messageID)
	if projectID == "" || messageID == "" {
		return 0, newInvalidInputError(map[string]string{"projectId": "cannot be empty", "messageId": "cannot be empty"})
	}
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return 0, err
	}
	filters, args := mentionFilter(actor)
	if filters == "" {
		return 0, nil
	}
	query := `
		UPDATE project_room_mentions m
		SET handled = TRUE,
			handled_at = NOW()
		FROM project_room_messages msg
		WHERE m.message_id = msg.id
		  AND msg.project_id = $1
		  AND msg.id = $2
		  AND m.handled = FALSE
		  AND (` + filters + `)
	`
	values := []any{projectID, messageID}
	values = append(values, args...)
	result, err := s.db.ExecContext(ctx, query, values...)
	if err != nil {
		return 0, fmt.Errorf("mark mentions handled failed: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

func (s *Service) PinMessage(ctx context.Context, actor identity.Identity, projectID string, messageID string, pinAs string) (Message, error) {
	if err := s.assertProjectWritable(ctx, projectID); err != nil {
		return Message{}, err
	}
	pinAs = strings.TrimSpace(pinAs)
	if pinAs == "" {
		pinAs = "decision"
	}
	_, err := s.GetMessage(ctx, projectID, messageID)
	if err != nil {
		return Message{}, err
	}
	operator := actor.OperatorLabel
	if !actor.IsOperator() {
		operator = s.operatorLabel
	}
	return s.SendSystemMessage(ctx, projectID, "memory_note", "Message pinned", map[string]any{
		"pinnedMessageId": messageID,
		"as":              pinAs,
		"pinnedBy":        operator,
	})
}

func (s *Service) TouchPresence(ctx context.Context, projectID string, actor identity.Identity, online bool) (Room, error) {
	room, err := s.GetRoom(ctx, projectID)
	if err != nil {
		return Room{}, err
	}
	if err := s.touchPresence(ctx, room.RoomID, projectID, actor, online); err != nil {
		return Room{}, err
	}
	return s.GetRoom(ctx, projectID)
}

func (s *Service) touchPresence(ctx context.Context, roomID string, projectID string, actor identity.Identity, online bool) error {
	key := ""
	if actor.IsAgent() {
		if strings.TrimSpace(actor.Agent.AgentID) == "" {
			return ErrRoomAccessDenied
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO project_room_members (
				id, room_id, project_id, member_type, agent_id, agent_type, role, last_seen_at
			) VALUES ($1,$2,$3,'agent',$4,$5,'member',NOW())
			ON CONFLICT (room_id, agent_id)
			DO UPDATE SET agent_type = EXCLUDED.agent_type, last_seen_at = EXCLUDED.last_seen_at
		`, ids.New("member"), roomID, projectID, actor.Agent.AgentID, nullableString(actor.Agent.AgentType))
		if err != nil {
			return fmt.Errorf("touch agent member failed: %w", err)
		}
		key = memberKey("agent", &actor.Agent.AgentID, nil)
	} else {
		label := strings.TrimSpace(actor.OperatorLabel)
		if label == "" {
			label = s.operatorLabel
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO project_room_members (
				id, room_id, project_id, member_type, operator_label, role, last_seen_at
			) VALUES ($1,$2,$3,'operator',$4,'owner',NOW())
			ON CONFLICT (room_id, operator_label)
			DO UPDATE SET last_seen_at = EXCLUDED.last_seen_at
		`, ids.New("member"), roomID, projectID, label)
		if err != nil {
			return fmt.Errorf("touch operator member failed: %w", err)
		}
		key = memberKey("operator", nil, &label)
	}

	s.onlineMu.Lock()
	if s.onlineMembers[projectID] == nil {
		s.onlineMembers[projectID] = map[string]time.Time{}
	}
	if online {
		s.onlineMembers[projectID][key] = s.now().UTC()
	} else {
		delete(s.onlineMembers[projectID], key)
	}
	if len(s.onlineMembers[projectID]) == 0 {
		delete(s.onlineMembers, projectID)
	}
	s.onlineMu.Unlock()

	s.syncPresenceStore(ctx, projectID, key, online)

	return nil
}

func (s *Service) isOnline(projectID string, key string) bool {
	s.onlineMu.RLock()
	defer s.onlineMu.RUnlock()
	project := s.onlineMembers[projectID]
	if project == nil {
		return false
	}
	_, ok := project[key]
	return ok
}

func (s *Service) syncPresenceStore(ctx context.Context, projectID string, key string, online bool) {
	if s.presenceStore == nil {
		return
	}
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(key) == "" {
		return
	}
	onlineKey := presenceOnlineKey(projectID)
	connectionsKey := presenceConnectionsKey(projectID)
	if online {
		if err := s.presenceStore.SAdd(ctx, onlineKey, key).Err(); err != nil {
			s.logger.Warn("presence store SADD failed", "projectId", projectID, "key", key, "error", err)
		}
		if err := s.presenceStore.HSet(ctx, connectionsKey, key, s.now().UTC().Unix()).Err(); err != nil {
			s.logger.Warn("presence store HSET failed", "projectId", projectID, "key", key, "error", err)
		}
		_ = s.presenceStore.Expire(ctx, onlineKey, 24*time.Hour).Err()
		_ = s.presenceStore.Expire(ctx, connectionsKey, 24*time.Hour).Err()
		return
	}
	if err := s.presenceStore.SRem(ctx, onlineKey, key).Err(); err != nil {
		s.logger.Warn("presence store SREM failed", "projectId", projectID, "key", key, "error", err)
	}
	if err := s.presenceStore.HDel(ctx, connectionsKey, key).Err(); err != nil {
		s.logger.Warn("presence store HDEL failed", "projectId", projectID, "key", key, "error", err)
	}
}

func (s *Service) assertProjectWritable(ctx context.Context, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return newInvalidInputError(map[string]string{"projectId": "cannot be empty"})
	}
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM projects WHERE id = $1`, projectID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRoomNotFound
	}
	if err != nil {
		return fmt.Errorf("load project writable state failed: %w", err)
	}
	if strings.TrimSpace(status) == "archived" {
		return ErrProjectArchived
	}
	return nil
}

func (s *Service) CleanupStalePresence(ctx context.Context, ttl time.Duration, limit int) (int, error) {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	if limit <= 0 {
		limit = 200
	}
	staleBefore := s.now().UTC().Add(-ttl)
	removed := 0

	s.onlineMu.Lock()
	for projectID, members := range s.onlineMembers {
		for key, lastSeen := range members {
			if lastSeen.After(staleBefore) {
				continue
			}
			delete(members, key)
			removed++
		}
		if len(members) == 0 {
			delete(s.onlineMembers, projectID)
		}
	}
	s.onlineMu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT project_id, member_type, COALESCE(agent_id, ''), COALESCE(operator_label, '')
		FROM project_room_members
		WHERE last_seen_at IS NOT NULL
		  AND last_seen_at < $1
		ORDER BY last_seen_at ASC
		LIMIT $2
	`, staleBefore, limit)
	if err != nil {
		return removed, fmt.Errorf("query stale room members failed: %w", err)
	}
	defer rows.Close()

	type staleMember struct {
		ProjectID     string
		MemberType    string
		AgentID       string
		OperatorLabel string
	}
	staleMembers := make([]staleMember, 0)
	for rows.Next() {
		var item staleMember
		if err := rows.Scan(&item.ProjectID, &item.MemberType, &item.AgentID, &item.OperatorLabel); err != nil {
			return removed, fmt.Errorf("scan stale room member failed: %w", err)
		}
		staleMembers = append(staleMembers, item)
	}
	if err := rows.Err(); err != nil {
		return removed, fmt.Errorf("iterate stale room members failed: %w", err)
	}
	for _, item := range staleMembers {
		var key string
		if item.MemberType == "agent" {
			agentID := strings.TrimSpace(item.AgentID)
			key = memberKey("agent", &agentID, nil)
		} else {
			operator := strings.TrimSpace(item.OperatorLabel)
			key = memberKey("operator", nil, &operator)
		}
		s.syncPresenceStore(ctx, item.ProjectID, key, false)
	}

	if s.presenceStore != nil {
		redisRemoved, err := s.cleanupPresenceStore(ctx, staleBefore, limit)
		if err != nil {
			return removed, err
		}
		removed += redisRemoved
	}

	return removed, nil
}

func (s *Service) cleanupPresenceStore(ctx context.Context, staleBefore time.Time, limit int) (int, error) {
	removed := 0
	cursor := uint64(0)
	for {
		keys, nextCursor, err := s.presenceStore.Scan(ctx, cursor, "room:presence:*:connections", int64(limit)).Result()
		if err != nil {
			return removed, fmt.Errorf("scan presence connections keys failed: %w", err)
		}
		for _, key := range keys {
			connectionsKey := strings.TrimSpace(key)
			if connectionsKey == "" {
				continue
			}
			projectID := projectIDFromConnectionsKey(connectionsKey)
			if projectID == "" {
				continue
			}
			entries, err := s.presenceStore.HGetAll(ctx, connectionsKey).Result()
			if err != nil {
				return removed, fmt.Errorf("presence HGETALL failed: %w", err)
			}
			for memberKey, unixText := range entries {
				lastSeenUnix, parseErr := strconv.ParseInt(strings.TrimSpace(unixText), 10, 64)
				if parseErr != nil {
					continue
				}
				if time.Unix(lastSeenUnix, 0).UTC().After(staleBefore) {
					continue
				}
				onlineKey := presenceOnlineKey(projectID)
				_ = s.presenceStore.HDel(ctx, connectionsKey, memberKey).Err()
				_ = s.presenceStore.SRem(ctx, onlineKey, memberKey).Err()
				removed++
			}
		}
		if nextCursor == 0 {
			break
		}
		cursor = nextCursor
	}
	return removed, nil
}

func (s *Service) GenerateDailySummaries(ctx context.Context, day time.Time, limit int) (int, error) {
	if s.memoryWriter == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	day = day.UTC()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	rows, err := s.db.QueryContext(ctx, `
		SELECT id
		FROM projects
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("list projects for daily summary failed: %w", err)
	}
	defer rows.Close()

	projectIDs := make([]string, 0)
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return 0, fmt.Errorf("scan project id for daily summary failed: %w", err)
		}
		projectIDs = append(projectIDs, projectID)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate project ids for daily summary failed: %w", err)
	}

	written := 0
	for _, projectID := range projectIDs {
		ok, err := s.writeDailySummaryForProject(ctx, projectID, start, end)
		if err != nil {
			return written, err
		}
		if ok {
			written++
		}
	}
	return written, nil
}

func (s *Service) writeDailySummaryForProject(ctx context.Context, projectID string, start time.Time, end time.Time) (bool, error) {
	title := "room-daily-summary-" + start.Format("2006-01-02")
	projectKey := projectID + ":" + start.Format("2006-01-02")

	if existing, err := s.memorySummaryExists(ctx, projectID, title); err == nil && existing {
		s.summaryMu.Lock()
		s.lastSummaryByID[projectKey] = s.now().UTC()
		s.summaryMu.Unlock()
		return false, nil
	}

	s.summaryMu.Lock()
	if !s.lastSummaryByID[projectKey].IsZero() {
		s.summaryMu.Unlock()
		return false, nil
	}
	s.summaryMu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT message_type, COALESCE(content, '')
		FROM project_room_messages
		WHERE project_id = $1
		  AND created_at >= $2
		  AND created_at < $3
		ORDER BY created_at DESC, id DESC
		LIMIT 120
	`, projectID, start, end)
	if err != nil {
		return false, fmt.Errorf("list room messages for daily summary failed: %w", err)
	}
	defer rows.Close()

	lines := []string{"# Room Daily Summary", "", "High-value updates:", ""}
	entries := 0
	for rows.Next() {
		var messageType string
		var content string
		if err := rows.Scan(&messageType, &content); err != nil {
			return false, fmt.Errorf("scan room message for daily summary failed: %w", err)
		}
		if strings.TrimSpace(messageType) == "text" {
			continue
		}
		text := strings.TrimSpace(content)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s", messageType, text))
		entries++
		if entries >= 20 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate room messages for daily summary failed: %w", err)
	}
	if entries == 0 {
		lines = append(lines, "- No significant non-text updates in this day window.")
	}

	if _, err := s.memoryWriter.Write(ctx, projectID, openviking.WriteInput{
		Target:   "summary",
		Title:    title,
		Content:  strings.Join(lines, "\n"),
		AutoSync: true,
	}); err != nil {
		s.logger.Warn("daily room summary write failed", "projectId", projectID, "day", start.Format("2006-01-02"), "error", err)
		return false, nil
	}

	s.summaryMu.Lock()
	s.lastSummaryByID[projectKey] = s.now().UTC()
	s.summaryMu.Unlock()
	return true, nil
}

func (s *Service) memorySummaryExists(ctx context.Context, projectID string, title string) (bool, error) {
	type memoryLister interface {
		List(ctx context.Context, projectID string) ([]openviking.ListItem, error)
	}
	lister, ok := s.memoryWriter.(memoryLister)
	if !ok {
		return false, nil
	}
	items, err := lister.List(ctx, projectID)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if strings.TrimSpace(item.Title) == strings.TrimSpace(title) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) Subscribe(projectID string) (<-chan Envelope, func()) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		ch := make(chan Envelope)
		close(ch)
		return ch, func() {}
	}
	return s.hub.subscribe(projectID)
}

func (s *Service) PublishTaskEvent(
	ctx context.Context,
	eventID string,
	projectID string,
	taskID string,
	eventType string,
	fromStatus string,
	toStatus string,
	_ identity.Identity,
	payload map[string]any,
	createdAt time.Time,
) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	eventPayload := map[string]any{
		"taskId":     taskID,
		"eventType":  eventType,
		"fromStatus": fromStatus,
		"toStatus":   toStatus,
		"details":    payload,
	}
	_, _ = s.SendSystemMessage(ctx, projectID, "system_event", fmt.Sprintf("Task %s updated: %s", taskID, eventType), eventPayload)
	roomID := s.roomIDOrEmpty(ctx, projectID)
	s.publish(projectID, Envelope{
		EventID:   nonEmpty(eventID, ids.New("evt")),
		EventType: "task.updated",
		ProjectID: projectID,
		RoomID:    roomID,
		Sender:    &MessageSender{Type: string(identity.SenderTypeSystem)},
		Payload:   eventPayload,
		CreatedAt: nonZeroTime(createdAt, s.now().UTC()),
	})
}

func (s *Service) PublishProjectCompleted(ctx context.Context, projectID string) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	payload := map[string]any{"status": "completed"}
	_, _ = s.SendSystemMessage(ctx, projectID, "system_event", "Project completed", payload)
	s.publish(projectID, Envelope{
		EventID:   ids.New("evt"),
		EventType: "task.updated",
		ProjectID: projectID,
		RoomID:    s.roomIDOrEmpty(ctx, projectID),
		Sender:    &MessageSender{Type: string(identity.SenderTypeSystem)},
		Payload: map[string]any{
			"eventType": "project.completed",
			"status":    "completed",
		},
		CreatedAt: s.now().UTC(),
	})
}

func (s *Service) PublishHandoffCreated(ctx context.Context, projectID string, handoffID string, taskID string) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return
	}
	payload := map[string]any{"handoffId": handoffID, "taskId": taskID}
	_, _ = s.SendSystemMessage(ctx, projectID, "context_handoff", "Context handoff created", payload)
	s.publish(projectID, Envelope{
		EventID:   ids.New("evt"),
		EventType: "context.handoff_created",
		ProjectID: projectID,
		RoomID:    s.roomIDOrEmpty(ctx, projectID),
		Sender:    &MessageSender{Type: string(identity.SenderTypeSystem)},
		Payload:   payload,
		CreatedAt: s.now().UTC(),
	})
}

func (s *Service) PublishPresence(projectID string, roomID string, eventType string, sender MessageSender, payload map[string]any) {
	projectID = strings.TrimSpace(projectID)
	roomID = strings.TrimSpace(roomID)
	if projectID == "" || roomID == "" {
		return
	}
	s.publish(projectID, s.newEnvelope(projectID, eventType, roomID, &sender, payload))
}

func SenderFromIdentity(actor identity.Identity, fallbackOperator string) MessageSender {
	senderType, senderOperator, senderAgentID, senderAgentType := senderColumns(actor, fallbackOperator)
	sender := MessageSender{Type: strings.TrimSpace(fmt.Sprint(senderType))}
	if sender.Type == "<nil>" || sender.Type == "" {
		sender.Type = string(identity.SenderTypeSystem)
	}
	if value, ok := senderOperator.(string); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			sender.OperatorLabel = &trimmed
		}
	}
	if value, ok := senderAgentID.(string); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			sender.AgentID = &trimmed
		}
	}
	if value, ok := senderAgentType.(string); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			sender.AgentType = &trimmed
		}
	}
	return sender
}

func (s *Service) publish(projectID string, envelope Envelope) {
	s.hub.publish(projectID, envelope)
}

func (s *Service) newEnvelope(projectID string, eventType string, roomID string, sender *MessageSender, payload any) Envelope {
	return Envelope{
		EventID:   ids.New("evt"),
		EventType: eventType,
		ProjectID: projectID,
		RoomID:    roomID,
		Sender:    sender,
		Payload:   payload,
		CreatedAt: s.now().UTC(),
	}
}

func (s *Service) insertMentions(ctx context.Context, roomID string, messageID string, content string) error {
	tokens := mentionPattern.FindAllStringSubmatch(content, -1)
	if len(tokens) == 0 {
		return nil
	}
	mentions := dedupeMentions(tokens)
	for _, mention := range mentions {
		var mentionedType any
		var mentionedID any
		var mentionedOperator any
		switch {
		case mention == "claude-code" || mention == "codex" || mention == "gemini":
			mentionedType = mention
		case strings.HasPrefix(mention, "agt_"):
			mentionedID = mention
		case mention == s.operatorLabel:
			mentionedOperator = mention
		default:
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO project_room_mentions (
				id, room_id, message_id, mentioned_agent_type, mentioned_agent_id, mentioned_operator_label
			) VALUES ($1,$2,$3,$4,$5,$6)
		`, ids.New("mention"), roomID, messageID, mentionedType, mentionedID, mentionedOperator); err != nil {
			return err
		}
	}
	return nil
}

func dedupeMentions(matches [][]string) []string {
	set := map[string]struct{}{}
	for _, item := range matches {
		if len(item) < 2 {
			continue
		}
		value := strings.TrimSpace(item[1])
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	if len(set) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func mentionFilter(actor identity.Identity) (string, []any) {
	if actor.IsAgent() {
		args := []any{}
		parts := []string{}
		agentID := strings.TrimSpace(actor.Agent.AgentID)
		agentType := strings.TrimSpace(actor.Agent.AgentType)
		if agentID != "" {
			parts = append(parts, fmt.Sprintf("m.mentioned_agent_id = $%d", len(args)+2))
			args = append(args, agentID)
		}
		if agentType != "" {
			parts = append(parts, fmt.Sprintf("(m.mentioned_agent_id IS NULL AND m.mentioned_agent_type = $%d)", len(args)+2))
			args = append(args, agentType)
		}
		return strings.Join(parts, " OR "), args
	}
	if actor.IsOperator() || actor.IsSystem() {
		label := strings.TrimSpace(actor.OperatorLabel)
		if label == "" {
			return "", nil
		}
		return "m.mentioned_operator_label = $2", []any{label}
	}
	return "", nil
}

func senderColumns(actor identity.Identity, fallbackOperator string) (senderType any, senderOperator any, senderAgentID any, senderAgentType any) {
	switch actor.SenderType {
	case identity.SenderTypeAgent:
		return "agent", nil, nullableString(actor.Agent.AgentID), nullableString(actor.Agent.AgentType)
	case identity.SenderTypeSystem:
		return "system", nil, nil, nil
	default:
		label := strings.TrimSpace(actor.OperatorLabel)
		if label == "" {
			label = fallbackOperator
		}
		return "operator", nullableString(label), nil, nil
	}
}

func scanMessage(scanner interface{ Scan(dest ...any) error }) (Message, error) {
	var message Message
	var senderType string
	var senderOperator sql.NullString
	var senderAgentID sql.NullString
	var senderAgentType sql.NullString
	var rawPayload []byte
	if err := scanner.Scan(
		&message.MessageID,
		&message.RoomID,
		&message.ProjectID,
		&senderType,
		&senderOperator,
		&senderAgentID,
		&senderAgentType,
		&message.MessageType,
		&message.Content,
		&rawPayload,
		&message.CreatedAt,
	); err != nil {
		return Message{}, err
	}
	message.Sender.Type = strings.TrimSpace(senderType)
	if senderOperator.Valid {
		value := strings.TrimSpace(senderOperator.String)
		if value != "" {
			message.Sender.OperatorLabel = &value
		}
	}
	if senderAgentID.Valid {
		value := strings.TrimSpace(senderAgentID.String)
		if value != "" {
			message.Sender.AgentID = &value
		}
	}
	if senderAgentType.Valid {
		value := strings.TrimSpace(senderAgentType.String)
		if value != "" {
			message.Sender.AgentType = &value
		}
	}
	if len(rawPayload) > 0 {
		_ = json.Unmarshal(rawPayload, &message.Payload)
	}
	if message.Payload == nil {
		message.Payload = map[string]any{}
	}
	return message, nil
}

func memberKey(memberType string, agentID *string, operatorLabel *string) string {
	if strings.TrimSpace(memberType) == "agent" {
		if agentID != nil {
			return "agent:" + strings.TrimSpace(*agentID)
		}
		return "agent:unknown"
	}
	if operatorLabel != nil {
		return "operator:" + strings.TrimSpace(*operatorLabel)
	}
	return "operator:unknown"
}

func normalizeMessageType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return value
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nonEmpty(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func nonZeroTime(value time.Time, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func presenceOnlineKey(projectID string) string {
	return "room:presence:" + strings.TrimSpace(projectID) + ":online"
}

func presenceConnectionsKey(projectID string) string {
	return "room:presence:" + strings.TrimSpace(projectID) + ":connections"
}

func projectIDFromConnectionsKey(key string) string {
	key = strings.TrimSpace(key)
	const prefix = "room:presence:"
	const suffix = ":connections"
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
		return ""
	}
	projectID := strings.TrimPrefix(key, prefix)
	projectID = strings.TrimSuffix(projectID, suffix)
	return strings.TrimSpace(projectID)
}

func (s *Service) roomIDOrEmpty(ctx context.Context, projectID string) string {
	var roomID string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM project_rooms WHERE project_id = $1`, projectID).Scan(&roomID); err != nil {
		return ""
	}
	return roomID
}

func (s *Service) maybeWriteRoomSummary(ctx context.Context, message Message) {
	if s.memoryWriter == nil {
		return
	}
	if message.MessageType == "text" || message.MessageType == "question" || message.MessageType == "answer" {
		return
	}
	projectKey := message.ProjectID + ":" + message.CreatedAt.UTC().Format("2006-01-02")
	s.summaryMu.Lock()
	last := s.lastSummaryByID[projectKey]
	now := s.now().UTC()
	if !last.IsZero() && now.Sub(last) < 10*time.Minute {
		s.summaryMu.Unlock()
		return
	}
	s.lastSummaryByID[projectKey] = now
	s.summaryMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		messages, _, err := s.ListMessages(ctx, message.ProjectID, 30, "")
		if err != nil {
			s.logger.Warn("room summary list failed", "projectId", message.ProjectID, "error", err)
			return
		}
		lines := []string{"# Room Daily Summary", "", "High-value updates:", ""}
		count := 0
		for _, item := range messages {
			if item.MessageType == "text" {
				continue
			}
			line := fmt.Sprintf("- [%s] %s", item.MessageType, strings.TrimSpace(item.Content))
			if strings.TrimSpace(item.Content) == "" {
				line = fmt.Sprintf("- [%s] messageId=%s", item.MessageType, item.MessageID)
			}
			lines = append(lines, line)
			count++
			if count >= 12 {
				break
			}
		}
		if count == 0 {
			lines = append(lines, "- No significant non-text updates in the latest window.")
		}
		title := "room-daily-summary-" + message.CreatedAt.UTC().Format("2006-01-02")
		_, err = s.memoryWriter.Write(ctx, message.ProjectID, openviking.WriteInput{
			Target:   "summary",
			Title:    title,
			Content:  strings.Join(lines, "\n"),
			AutoSync: true,
		})
		if err != nil {
			s.logger.Warn("room summary write failed", "projectId", message.ProjectID, "error", err)
		}
	}()
}
