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
	c.bootstrapping = true
	h.rooms[c.roomID][c] = struct{}{}
}

// finishBootstrap places server.ready before every event received while the
// initial state snapshot was loading. Without this gate, a status.changed
// event could arrive before ready and then be overwritten by an older snapshot.
func (h *hub) finishBootstrap(c *client, ready []byte) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[c.roomID][c]; !ok || len(c.send)+len(c.pending)+1 > cap(c.send) {
		return false
	}
	c.send <- ready
	for _, message := range c.pending {
		c.send <- message
	}
	c.pending = nil
	c.bootstrapping = false
	return true
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c.pending = nil
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
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[roomID] {
		if c.bootstrapping {
			// Keep one slot available for server.ready so the bootstrap order
			// remains lossless and deterministic.
			if len(c.pending)+len(c.send) >= cap(c.send)-1 {
				c.cancel()
				continue
			}
			c.pending = append(c.pending, message)
			continue
		}
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
