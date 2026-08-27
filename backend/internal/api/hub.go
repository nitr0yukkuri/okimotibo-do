package api

import "sync"

type hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*client]struct{}
}

func newHub() *hub { return &hub{rooms: make(map[string]map[*client]struct{})} }

func (h *hub) add(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[c.roomID] == nil {
		h.rooms[c.roomID] = make(map[*client]struct{})
	}
	h.rooms[c.roomID][c] = struct{}{}
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[c.roomID], c)
	if len(h.rooms[c.roomID]) == 0 {
		delete(h.rooms, c.roomID)
	}
}

func (h *hub) hasPublisher(roomID, userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[roomID] {
		if !c.readOnly && c.userID == userID {
			return true
		}
	}
	return false
}

func (h *hub) broadcast(roomID string, message []byte) {
	h.mu.RLock()
	clients := make([]*client, 0, len(h.rooms[roomID]))
	for c := range h.rooms[roomID] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		select {
		case c.send <- message:
		default:
			c.cancel()
		}
	}
}

func (h *hub) close() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, room := range h.rooms {
		for c := range room {
			c.cancel()
		}
	}
}
