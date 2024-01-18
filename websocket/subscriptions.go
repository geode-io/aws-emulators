package websocket

type Listener struct {
	// ID of the listener in the emulated API gateway
	ID string

	// The handler for messages
	OnMessage func(Msg)

	// The handler for connections
	OnConnect func(Connection)

	// The handler for disconnections
	OnDisconnect func(Connection)
}
