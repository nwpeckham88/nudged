package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"nudged/internal/hub"
)

// Start runs the server core. It returns when the provided context is
// cancelled (graceful shutdown).
func Start(ctx context.Context, addr string) error {
	fmt.Println("starting server and hub")
	h := hub.New()

	// quick smoke: subscribe and publish
	sub, unsub := h.Subscribe("_internal_smoke")
	defer unsub()

	go func() {
		time.Sleep(10 * time.Millisecond)
		h.Publish(hub.Message{Topic: "_internal_smoke", Payload: "ok", From: "server"})
	}()

	select {
	case msg := <-sub:
		fmt.Printf("hub smoke received: %v\n", msg.Payload)
	case <-time.After(500 * time.Millisecond):
		fmt.Println("hub smoke timeout")
	}

	// start HTTP status endpoint
	// simple in-memory registry for Agents and Apps
	type Agent struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Addr     string   `json:"addr"`
		Apps     []string `json:"apps"`
		LastSeen int64    `json:"last_seen"`
	}

	type Registry struct {
		mu     sync.RWMutex
		agents map[string]*Agent
	}

	reg := &Registry{agents: make(map[string]*Agent)}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// POST /agents - register or update an agent
	mux.HandleFunc("/agents", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var a Agent
			if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			a.LastSeen = time.Now().Unix()
			reg.mu.Lock()
			reg.agents[a.ID] = &a
			reg.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		case http.MethodGet:
			reg.mu.RLock()
			list := make([]*Agent, 0, len(reg.agents))
			for _, v := range reg.agents {
				list = append(list, v)
			}
			reg.mu.RUnlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(list)
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
	})

	// GET /apps - list known apps and which agents advertise them
	mux.HandleFunc("/apps", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		reg.mu.RLock()
		apps := make(map[string][]string)
		for _, a := range reg.agents {
			for _, app := range a.Apps {
				apps[app] = append(apps[app], a.ID)
			}
		}
		reg.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apps)
	})

	// WebSocket control-plane: /ws/register
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	secret := os.Getenv("NUDGED_HUB_SECRET")
	mux.HandleFunc("/ws/register", func(w http.ResponseWriter, r *http.Request) {
		if secret != "" {
			key := r.Header.Get("X-Nudged-Secret")
			if key != secret {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// expect first JSON message to contain agent identity
		var a struct {
			ID   string   `json:"id"`
			Name string   `json:"name"`
			Addr string   `json:"addr"`
			Apps []string `json:"apps"`
		}
		if err := conn.ReadJSON(&a); err != nil {
			conn.Close()
			return
		}

		ag := &Agent{ID: a.ID, Name: a.Name, Addr: a.Addr, Apps: a.Apps, LastSeen: time.Now().Unix()}
		reg.mu.Lock()
		reg.agents[ag.ID] = ag
		reg.mu.Unlock()

		// keep connection alive and update LastSeen; remove on close
		go func(id string, c *websocket.Conn) {
			defer func() {
				c.Close()
				reg.mu.Lock()
				delete(reg.agents, id)
				reg.mu.Unlock()
			}()

			for {
				var msg map[string]any
				if err := c.ReadJSON(&msg); err != nil {
					return
				}
				reg.mu.Lock()
				if ag, ok := reg.agents[id]; ok {
					ag.LastSeen = time.Now().Unix()
				}
				reg.mu.Unlock()
			}
		}(ag.ID, conn)
	})

	srv := &http.Server{Addr: addr, Handler: mux}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// start background activity to demonstrate a long-running process
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("shutting down server")
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutCtx)
			return nil
		case err := <-serverErr:
			if err != nil {
				return err
			}
		case <-ticker.C:
			// heartbeat publish (non-blocking to subscribers)
			h.Publish(hub.Message{Topic: "heartbeat", Payload: time.Now(), From: "server"})
		}
	}
}
