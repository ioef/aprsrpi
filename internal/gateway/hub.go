package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"aprsrpi/internal/aprs"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[chan aprs.Message]struct{}
	history []aprs.Message
	nextID  int64
}

func NewHub() *Hub { return &Hub{clients: make(map[chan aprs.Message]struct{})} }
func (h *Hub) Publish(message aprs.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	message.ID = h.nextID
	h.nextID++
	h.history = append([]aprs.Message{message}, h.history...)
	if len(h.history) > 100 {
		h.history = h.history[:100]
	}
	for client := range h.clients {
		select {
		case client <- message:
		default:
		}
	}
}
func (h *Hub) Snapshot() []aprs.Message {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]aprs.Message(nil), h.history...)
}
func (h *Hub) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	client := make(chan aprs.Message, 8)
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()
	defer func() { h.mu.Lock(); delete(h.clients, client); close(client); h.mu.Unlock() }()
	for {
		select {
		case message := <-client:
			data, _ := json.Marshal(message)
			_, _ = fmt.Fprintf(w, "event: aprs\ndata: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
