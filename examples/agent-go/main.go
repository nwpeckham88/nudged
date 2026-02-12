package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	hubAddr := env("HUB_ADDR", "ws://hub:8080/ws/register")
	agentID := env("AGENT_ID", "agent-local")
	agentName := env("AGENT_NAME", "agent-local")
	agentAddr := env("AGENT_ADDR", "127.0.0.1:59999")
	apps := env("AGENT_APPS", "demo")

	// start a small health endpoint for the agent
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "status": "ok"})
		})
		log.Println("agent health listening on :8081")
		if err := http.ListenAndServe(":8081", nil); err != nil {
			log.Printf("health server: %v", err)
		}
	}()

	for {
		u, err := url.Parse(hubAddr)
		if err != nil {
			log.Printf("invalid HUB_ADDR %s: %v", hubAddr, err)
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("connecting to %s", u.String())
		c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			log.Printf("dial error: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		ident := map[string]any{"id": agentID, "name": agentName, "addr": agentAddr, "apps": []string{apps}}
		if err := c.WriteJSON(ident); err != nil {
			log.Printf("write ident failed: %v", err)
			c.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		fmt.Printf("registered: %v\n", ident)

		// read loop
		for {
			var v any
			if err := c.ReadJSON(&v); err != nil {
				log.Printf("read error: %v", err)
				break
			}
			b, _ := json.MarshalIndent(v, "", "  ")
			fmt.Printf("recv: %s\n", string(b))
		}

		c.Close()
		time.Sleep(2 * time.Second)
	}
}
