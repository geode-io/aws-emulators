package websocket

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Connection tracks the underlying websocket, as well as its ID for API gateway callback purposes
type Connection struct {
	// ID of the connection in the emulated API gateway
	ID string
	// The websocket hub that the client is connected to.
	hub *Hub
	// The websocket connection.
	ws *websocket.Conn
	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Connection) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	err := c.ws.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		zap.L().Warn("failed to set read deadline", zap.Error(err))
		return
	}
	c.ws.SetPongHandler(func(string) error { return c.ws.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				zap.L().Error("websocket connection closed unexpectedly", zap.Error(err))
				// will be unregistered in defer
			}
			break
		}
		message = bytes.TrimSpace(bytes.ReplaceAll(message, newline, space))
		c.hub.inbound <- &Msg{
			ConnectionID: c.ID,
			Data:         message,
		}
	}
}

func (c *Connection) onSend(message []byte, hasMessage bool) error {
	_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	if !hasMessage {
		// The hub closed the channel.
		err := c.ws.WriteMessage(websocket.CloseMessage, []byte{})
		if err != nil {
			return fmt.Errorf("failed to write close message: %w", err)
		}
		return nil
	}

	w, err := c.ws.NextWriter(websocket.TextMessage)
	if err != nil {
		return fmt.Errorf("failed to get next writer: %w", err)
	}
	_, err = w.Write(message)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Add queued chat messages to the current websocket message.
	n := len(c.send)
	for i := 0; i < n; i++ {
		_, err = w.Write(newline)
		if err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
		_, err = w.Write(<-c.send)
		if err != nil {
			return fmt.Errorf("failed to write message: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	return nil
}

func (c *Connection) keepAlive() error {
	_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
		return fmt.Errorf("failed to write ping message: %w", err)
	}

	return nil
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if err := c.onSend(message, ok); err != nil {
				zap.L().Error("failed to send message", zap.Error(err))
				return
			}
		case <-ticker.C:
			if err := c.keepAlive(); err != nil {
				zap.L().Warn("failed to send keep alive", zap.Error(err))
				continue
			}
		}
	}
}

func serveWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	connectionID := uuid.New().String()
	zap.L().Info("received websocket connection", zap.String("connection.id", connectionID))
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		zap.L().Error("failed to upgrade websocket connection", zap.Error(err))
		return
	}
	conn := &Connection{
		ID:   connectionID,
		hub:  hub,
		ws:   ws,
		send: make(chan []byte, 256),
	}
	conn.hub.register <- conn

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go conn.writePump()
	go conn.readPump()
}
