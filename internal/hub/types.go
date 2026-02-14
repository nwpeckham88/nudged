package hub

import "github.com/gorilla/websocket"

// Agent represents a connected agent.
type Agent struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Addr     string          `json:"addr"`
	Apps     []string        `json:"apps"`
	LastSeen int64           `json:"last_seen"`
	Conn     *websocket.Conn `json:"-"`
}
