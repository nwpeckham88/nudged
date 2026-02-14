package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
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
	mu        sync.RWMutex
	reconnect time.Duration
	logger    *slog.Logger
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	
	var d Docker

	if cfg.Mock {
		d = &MockDocker{}
		logger.Info("Forcing Mock Docker client via config")
	} else {
		var err error
		d, err = NewDockerClient()
		if err != nil {
			logger.Warn("failed to create docker client, using mock", "error", err)
			d = &MockDocker{}
		} else {
			// Test connection
			apps, err := d.Scan(context.Background())
			if err != nil {
				logger.Warn("Docker client failed, falling back to mock for testing", "error", err)
				d = &MockDocker{}
			} else {
				logger.Info("Docker client connected", "apps_found", len(apps))
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
		logger:    logger.With("component", "agent", "agent_id", cfg.ID),
	}, nil
}

// Run starts the agent loop.
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("starting agent", "name", a.Name)

	// Start server (health + proxy)
	go a.serve()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := a.connectAndServe(ctx); err != nil {
				a.logger.Error("connection error", "error", err)
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

func (a *Agent) serve() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": a.ID, "status": "ok"})
	})

	// Proxy logic: forward requests to the appropriate container
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}

		// assume subdomain matches app name
		parts := strings.Split(host, ".")
		appName := parts[0]

		a.mu.RLock()
		app, ok := a.Apps[appName]
		a.mu.RUnlock()

		if !ok {
			http.Error(w, "app not found", http.StatusNotFound)
			return
		}

		targetURL := &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%s", app.ContainerName, app.Port),
		}

		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			a.logger.Error("proxy error", "app", appName, "error", err)
			http.Error(rw, "bad gateway", http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	})

	a.logger.Info("agent server listening", "addr", a.Addr)
	if err := http.ListenAndServe(a.Addr, mux); err != nil {
		a.logger.Error("agent server failed", "error", err)
	}
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	// Scan for apps first
	apps, err := a.Docker.Scan(ctx)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	
	appNames := make([]string, 0, len(apps))
	
	a.mu.Lock()
	a.Apps = make(map[string]App)
	for _, app := range apps {
		appNames = append(appNames, app.Name)
		a.Apps[app.Name] = app
	}
	a.mu.Unlock()
	
	a.logger.Info("found apps", "apps", appNames)

	u, err := url.Parse(a.HubAddr)
	if err != nil {
		return fmt.Errorf("invalid hub addr: %w", err)
	}

	header := http.Header{}
	if a.Secret != "" {
		header.Set("X-Nudged-Secret", a.Secret)
	}

	a.logger.Info("connecting to hub", "url", u.String())
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
	a.logger.Info("registered with hub")

	// Read loop
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var msg map[string]any
			if err := c.ReadJSON(&msg); err != nil {
				a.logger.Error("read error", "error", err)
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
	a.logger.Debug("recv", "msg", msg)
	
	typ, _ := msg["type"].(string)
	switch typ {
	case "wake":
		appName, _ := msg["app"].(string)
		if app, ok := a.Apps[appName]; ok {
			go a.wakeApp(ctx, c, app)
		} else {
			a.logger.Warn("unknown app to wake", "app", appName)
		}
	}
}

func (a *Agent) wakeApp(ctx context.Context, c *websocket.Conn, app App) {
	a.logger.Info("waking app", "app", app.Name, "container_id", app.ContainerID)
	
	// Send STARTING status
	_ = c.WriteJSON(map[string]any{
		"type": "status",
		"app":  app.Name,
		"state": "STARTING",
	})

	if err := a.Docker.StartContainer(ctx, app.ContainerID); err != nil {
		a.logger.Error("failed to start container", "app", app.Name, "error", err)
		_ = c.WriteJSON(map[string]any{
			"type": "status",
			"app":  app.Name,
			"state": "ERROR",
			"error": err.Error(),
		})
		return
	}

	// Wait for readiness (check TCP port)
	for i := 0; i < 30; i++ {
		// First check if container is running
		running, err := a.Docker.IsRunning(ctx, app.ContainerID)
		if err == nil && running {
			// Then check if port is open
			if a.checkPort(ctx, app) {
				// Send READY status
				a.logger.Info("app is ready", "app", app.Name)
				_ = c.WriteJSON(map[string]any{
					"type": "status",
					"app":  app.Name,
					"state": "READY",
					"port":  app.Port,
				})
				return
			}
		}
		time.Sleep(1 * time.Second)
	}

	// Timeout
	a.logger.Warn("app wake timeout", "app", app.Name)
	_ = c.WriteJSON(map[string]any{
		"type": "status",
		"app":  app.Name,
		"state": "TIMEOUT",
	})
}

func (a *Agent) checkPort(ctx context.Context, app App) bool {
	target := fmt.Sprintf("%s:%s", app.ContainerName, app.Port)
	// If running in Mock mode, return true
	if _, ok := a.Docker.(*MockDocker); ok {
		return true
	}

	d := net.Dialer{Timeout: 1 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", target)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
