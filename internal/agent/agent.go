package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// Agent manages the connection to the Hub and local Docker containers.
type Agent struct {
	ID        string
	Name      string
	Addr      string
	HubAddr   string
	Secret    string
	Docker    Docker
	Apps      map[string]App
	reconnect time.Duration
}

// Config holds configuration for the Agent.
type Config struct {
	ID        string
	Name      string
	Addr      string
	HubAddr   string
	Secret    string
	Mock      bool
}

// New creates a new Agent.
func New(cfg Config) (*Agent, error) {
	var d Docker

	if cfg.Mock {
		d = &MockDocker{}
		log.Println("Forcing Mock Docker client via config")
	} else {
		var err error
		d, err = NewDockerClient()
		if err != nil {
			log.Printf("failed to create docker client: %v, using mock", err)
			d = &MockDocker{}
		} else {
			// Test connection
			apps, err := d.Scan(context.Background())
			if err != nil {
				log.Printf("Docker client failed (%v), falling back to mock for testing", err)
				d = &MockDocker{}
			} else {
				log.Printf("Docker client connected. Found %d apps.", len(apps))
			}
		}
	}

	return &Agent{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Addr:      cfg.Addr,
		HubAddr:   cfg.HubAddr,
		Secret:    cfg.Secret,
		Docker:    d,
		Apps:      make(map[string]App),
		reconnect: 3 * time.Second,
	}, nil
}

// Run starts the agent loop.
func (a *Agent) Run(ctx context.Context) error {
	log.Printf("starting agent %s (%s)", a.Name, a.ID)

	// Start health server
	go a.serveHealth()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := a.connectAndServe(ctx); err != nil {
				log.Printf("connection error: %v", err)
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(a.reconnect):
				continue
			}
		}
	}
}

func (a *Agent) serveHealth() {
	// extract port from Addr
	// ... simple implementation for now
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": a.ID, "status": "ok"})
	})
	
	// Assume Addr is host:port, just listen on it
	log.Printf("health listening on %s", a.Addr)
	if err := http.ListenAndServe(a.Addr, mux); err != nil {
		log.Printf("health server failed: %v", err)
	}
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	// Scan for apps first
	apps, err := a.Docker.Scan(ctx)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	
	appNames := make([]string, 0, len(apps))
	a.Apps = make(map[string]App)
	for _, app := range apps {
		appNames = append(appNames, app.Name)
		a.Apps[app.Name] = app
	}
	log.Printf("found apps: %v", appNames)

	u, err := url.Parse(a.HubAddr)
	if err != nil {
		return fmt.Errorf("invalid hub addr: %w", err)
	}

	header := http.Header{}
	if a.Secret != "" {
		header.Set("X-Nudged-Secret", a.Secret)
	}

	log.Printf("connecting to %s", u.String())
	c, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if err != nil {
		return err
	}
	defer c.Close()

	// Register
	ident := map[string]any{
		"id":   a.ID,
		"name": a.Name,
		"addr": a.Addr,
		"apps": appNames,
	}
	if err := c.WriteJSON(ident); err != nil {
		return fmt.Errorf("write ident failed: %w", err)
	}
	log.Printf("registered with hub")

	// Read loop
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg map[string]any
			if err := c.ReadJSON(&msg); err != nil {
				log.Printf("read error: %v", err)
				return
			}
			a.handleMessage(ctx, c, msg)
		}
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		return nil
	}
}

func (a *Agent) handleMessage(ctx context.Context, c *websocket.Conn, msg map[string]any) {
	log.Printf("recv: %v", msg)
	
	typ, _ := msg["type"].(string)
	switch typ {
	case "wake":
		appName, _ := msg["app"].(string)
		if app, ok := a.Apps[appName]; ok {
			go a.wakeApp(ctx, c, app)
		} else {
			log.Printf("unknown app to wake: %s", appName)
		}
	}
}

func (a *Agent) wakeApp(ctx context.Context, c *websocket.Conn, app App) {
	log.Printf("waking app %s (container %s)...", app.Name, app.ContainerID)
	
	// Send STARTING status
	_ = c.WriteJSON(map[string]any{
		"type": "status",
		"app":  app.Name,
		"state": "STARTING",
	})

	if err := a.Docker.StartContainer(ctx, app.ContainerID); err != nil {
		log.Printf("failed to start container %s: %v", app.Name, err)
		_ = c.WriteJSON(map[string]any{
			"type": "status",
			"app":  app.Name,
			"state": "ERROR",
			"error": err.Error(),
		})
		return
	}

	// Wait for readiness (simple sleep + check running for now, eventually curl)
	// TODO: Implement actual port check
	for i := 0; i < 30; i++ {
		running, err := a.Docker.IsRunning(ctx, app.ContainerID)
		if err == nil && running {
			// Send READY status
			log.Printf("app %s is ready", app.Name)
			_ = c.WriteJSON(map[string]any{
				"type": "status",
				"app":  app.Name,
				"state": "READY",
				"port":  app.Port,
			})
			return
		}
		time.Sleep(1 * time.Second)
	}

	// Timeout
	_ = c.WriteJSON(map[string]any{
		"type": "status",
		"app":  app.Name,
		"state": "TIMEOUT",
	})
}
