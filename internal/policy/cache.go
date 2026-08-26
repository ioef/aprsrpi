package policy

import (
	"crypto/sha1"
	"encoding/hex"
	"sync"
	"time"
)

type Cache struct {
	mu     sync.Mutex
	values map[string]time.Time
	ttl    time.Duration
}

type Heard struct {
	mu     sync.Mutex
	values map[string]time.Time
	ttl    time.Duration
}

type Limiter struct {
	mu   sync.Mutex
	next map[string]time.Time
}

type WindowLimiter struct {
	mu     sync.Mutex
	events map[string][]time.Time
}

func NewWindowLimiter() *WindowLimiter { return &WindowLimiter{events: make(map[string][]time.Time)} }
func (l *WindowLimiter) Allow(key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return false
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	events := l.events[key][:0]
	cutoff := now.Add(-window)
	for _, event := range l.events[key] {
		if event.After(cutoff) {
			events = append(events, event)
		}
	}
	if len(events) >= limit {
		l.events[key] = events
		return false
	}
	l.events[key] = append(events, now)
	return true
}

func NewLimiter() *Limiter { return &Limiter{next: make(map[string]time.Time)} }
func (l *Limiter) Allow(key string, interval time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if next, ok := l.next[key]; ok && now.Before(next) {
		return false
	}
	l.next[key] = now.Add(interval)
	return true
}

func NewHeard(ttl time.Duration) *Heard { return &Heard{values: make(map[string]time.Time), ttl: ttl} }
func (h *Heard) Mark(call string)       { h.mu.Lock(); h.values[call] = time.Now(); h.mu.Unlock() }
func (h *Heard) Recent(call string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	at, ok := h.values[call]
	return ok && time.Since(at) <= h.ttl
}

func NewCache(ttl time.Duration) *Cache { return &Cache{values: make(map[string]time.Time), ttl: ttl} }
func (c *Cache) Seen(value string) bool {
	key := fingerprint(value)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, at := range c.values {
		if now.Sub(at) > c.ttl {
			delete(c.values, key)
		}
	}
	if _, exists := c.values[key]; exists {
		return true
	}
	c.values[key] = now
	return false
}
func fingerprint(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
