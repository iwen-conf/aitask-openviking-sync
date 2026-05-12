package room

import "sync"

type hub struct {
	mu      sync.RWMutex
	nextID  int
	clients map[string]map[int]chan Envelope
}

func newHub() *hub {
	return &hub{clients: map[string]map[int]chan Envelope{}}
}

func (h *hub) subscribe(projectID string) (<-chan Envelope, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextID++
	id := h.nextID
	if h.clients[projectID] == nil {
		h.clients[projectID] = map[int]chan Envelope{}
	}
	ch := make(chan Envelope, 32)
	h.clients[projectID][id] = ch

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		projectClients := h.clients[projectID]
		if projectClients == nil {
			return
		}
		if c, ok := projectClients[id]; ok {
			delete(projectClients, id)
			close(c)
		}
		if len(projectClients) == 0 {
			delete(h.clients, projectID)
		}
	}
	return ch, cancel
}

func (h *hub) publish(projectID string, envelope Envelope) {
	h.mu.RLock()
	projectClients := h.clients[projectID]
	if len(projectClients) == 0 {
		h.mu.RUnlock()
		return
	}
	clients := make([]chan Envelope, 0, len(projectClients))
	for _, ch := range projectClients {
		clients = append(clients, ch)
	}
	h.mu.RUnlock()

	for _, ch := range clients {
		select {
		case ch <- envelope:
		default:
			// Drop when subscriber is slow; websocket reconnect will resync via HTTP APIs.
		}
	}
}
