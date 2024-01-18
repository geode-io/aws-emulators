package websocket

import (
	"context"
	"net/http"
)

type Msg struct {
	ConnectionID string
	Data         []byte
}

type Hub struct {
	// Registered connections.
	connections map[string]*Connection

	// Registered listeners.
	listeners []*Listener

	// Inbound messages from the connections.
	inbound chan *Msg

	// Outbound messages to the connections.
	outbound chan *Msg

	// Inbound registration requests from new connections.
	register chan *Connection

	// Inbound de-registration requests from expiring connections.
	unregister chan *Connection

	// Inbound listen requests from new listeners.
	listen chan *Listener
}

func NewHub() *Hub {
	return &Hub{
		inbound:     make(chan *Msg),
		outbound:    make(chan *Msg),
		register:    make(chan *Connection),
		unregister:  make(chan *Connection),
		connections: make(map[string]*Connection),
		listen:      make(chan *Listener),
	}
}

func (h *Hub) RegisterListener(listener *Listener) {
	h.listen <- listener
}

func (h *Hub) SendOutboundMessage(msg *Msg) {
	h.outbound <- msg
}

func (h *Hub) HasConnection(connectionID string) bool {
	_, ok := h.connections[connectionID]
	return ok
}

func (h *Hub) ServeRequest(w http.ResponseWriter, req *http.Request) {
	serveWS(h, w, req)
}

//nolint:funlen,gocognit
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case listener := <-h.listen:
			h.listeners = append(h.listeners, listener)
		case connection := <-h.register:
			h.connections[connection.ID] = connection
			for _, listener := range h.listeners {
				if listener.OnConnect == nil {
					continue
				}
				listener.OnConnect(*connection)
			}
		case connection := <-h.unregister:
			if _, ok := h.connections[connection.ID]; ok {
				delete(h.connections, connection.ID)
				close(connection.send)
			}
			for _, listener := range h.listeners {
				if listener.OnDisconnect == nil {
					continue
				}
				listener.OnDisconnect(*connection)
			}
		case message := <-h.inbound:
			if message == nil {
				continue
			}
			for _, listener := range h.listeners {
				if listener.OnMessage == nil {
					continue
				}
				listener.OnMessage(*message)
			}
		case message := <-h.outbound:
			if connection, ok := h.connections[message.ConnectionID]; ok {
				select {
				case connection.send <- message.Data:
				default:
					close(connection.send)
					delete(h.connections, connection.ID)
				}
			}
		}
	}
}
