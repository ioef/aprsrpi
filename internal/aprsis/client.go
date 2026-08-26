package aprsis

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"aprsrpi/internal/logging"
)

type Config struct {
	Enabled  bool
	Server   string
	Callsign string
	Passcode string
	Filter   string
}
type Client struct {
	config     Config
	mu         sync.RWMutex
	writeMu    sync.Mutex
	connection net.Conn
	ready      bool
}

func New(config Config) *Client { return &Client{config: config} }

func (c *Client) Run(ctx context.Context, receive func(string)) {
	if !c.config.Enabled || c.config.Server == "" {
		return
	}
	for {
		if err := c.session(ctx, receive); err != nil && ctx.Err() == nil {
			logging.Warnf("APRS-IS connection: %v", err)
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *Client) session(ctx context.Context, receive func(string)) error {
	connection, err := net.DialTimeout("tcp", c.config.Server, 10*time.Second)
	if err != nil {
		return err
	}
	logging.Infof("aprs-is connected server=%s", c.config.Server)
	c.mu.Lock()
	c.connection = connection
	c.ready = false
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.connection == connection {
			c.connection = nil
			c.ready = false
		}
		c.mu.Unlock()
		_ = connection.Close()
	}()
	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-closed:
		}
	}()
	login := fmt.Sprintf("user %s pass %s vers aprsrpi 0.1", c.config.Callsign, c.config.Passcode)
	if c.config.Filter != "" {
		login += " filter " + c.config.Filter
	}
	logging.Infof("aprs-is login callsign=%s filter=%q", c.config.Callsign, c.config.Filter)
	if _, err := fmt.Fprintln(connection, login); err != nil {
		return err
	}
	_ = connection.SetReadDeadline(time.Now().Add(15 * time.Second))
	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)
	heartbeatStarted := false
	scanner := bufio.NewScanner(connection)
	scanner.Buffer(make([]byte, 1024), 8192)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# logresp") {
			if !strings.Contains(strings.ToLower(line), "verified") {
				logging.Errorf("aprs-is login rejected response=%q", line)
				return fmt.Errorf("APRS-IS login was not verified: %s", line)
			}
			c.mu.Lock()
			if c.connection == connection {
				c.ready = true
			}
			c.mu.Unlock()
			logging.Infof("aprs-is login verified callsign=%s", c.config.Callsign)
			if !heartbeatStarted {
				_ = connection.SetReadDeadline(time.Time{})
				go c.heartbeat(connection, stopHeartbeat)
				heartbeatStarted = true
			}
			continue
		}
		logging.Debugf("aprs-is server line=%q", line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		receive(line)
	}
	return scanner.Err()
}

func (c *Client) heartbeat(connection net.Conn, stop <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.writeMu.Lock()
			_, _ = io.WriteString(connection, "# aprsrpi heartbeat\r\n")
			c.writeMu.Unlock()
		}
	}
}

func (c *Client) Send(packet string) error {
	c.mu.RLock()
	connection := c.connection
	ready := c.ready
	c.mu.RUnlock()
	if connection == nil || !ready {
		return fmt.Errorf("APRS-IS is not connected")
	}
	packet = strings.TrimRight(packet, "\r\n")
	if len(packet) > 511 {
		return fmt.Errorf("APRS-IS packet exceeds 511 bytes")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := io.WriteString(connection, packet+"\r\n")
	if err != nil {
		logging.Warnf("aprs-is send failed: %v", err)
	} else {
		logging.Infof("aprs-is send packet=%q", packet)
	}
	return err
}
