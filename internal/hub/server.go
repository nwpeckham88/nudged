package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	webui "github.com/nwpeckham88/nudged/internal/webui"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	hubConnectedAgents = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nudged_hub_connected_agents",
		Help: "Number of currently connected agents",
	})
	hubRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nudged_hub_requests_total",
		Help: "Total HTTP requests handled by Hub",
	})
	hubWakeRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nudged_hub_wake_requests_total",
		Help: "Total wake requests triggered",
	})
)

// Start runs the server core. It returns when the provided context is
// cancelled (graceful shutdown).
func Start(ctx context.Context, addr string) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("starting server and hub", "version", "0.1.0")

	// Initialize Store
	store, err := NewStore("nudged.db")
	if err != nil {
		logger.Error("failed to init store", "error", err)
		return err
	}

	h := New()

	// quick smoke: subscribe and publish
	sub, unsub := h.Subscribe("_internal_smoke")
	defer unsub()

	go func() {
		time.Sleep(10 * time.Millisecond)
		h.Publish(Message{Topic: "_internal_smoke", Payload: "ok", From: "server"})
	}()

	select {
	case msg := <-sub:
		logger.Debug("hub smoke received", "payload", msg.Payload)
	case <-time.After(500 * time.Millisecond):
		logger.Warn("hub smoke timeout")
	}

	// start HTTP status endpoint
	// simple in-memory registry for Agents and Apps
	// Agent struct is now defined in types.go (to be created) or we move it to package scope in store.go
	// For now, let's keep it here but we need to make it accessible to store.go or vice versa.
	// Since store.go is in the same package, we can move the Agent struct definition to a common place or top level.

	type Registry struct {
		mu     sync.RWMutex
		agents map[string]*Agent
	}

	// Load existing agents from store
	storedAgents, err := store.LoadAgents()
	if err != nil {
		logger.Error("failed to load agents", "error", err)
	} else {
		logger.Info("loaded agents from store", "count", len(storedAgents))
	}
	if storedAgents == nil {
		storedAgents = make(map[string]*Agent)
	}

	reg := &Registry{agents: storedAgents}

	mux := http.NewServeMux()
	// serve embedded web UI at /ui/
	webui.Register(mux, "/ui/")
	// health endpoints: /healthz and /health
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

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
			
			// Persist
			if err := store.SaveAgent(&a); err != nil {
				logger.Error("failed to save agent", "error", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			reg.mu.Lock()
			reg.agents[a.ID] = &a
			hubConnectedAgents.Set(float64(len(reg.agents)))
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

		ag := &Agent{ID: a.ID, Name: a.Name, Addr: a.Addr, Apps: a.Apps, LastSeen: time.Now().Unix(), Conn: conn}
		
		// Persist
		if err := store.SaveAgent(ag); err != nil {
			logger.Error("failed to save agent", "error", err)
			conn.Close()
			return
		}

		reg.mu.Lock()
		reg.agents[ag.ID] = ag
		hubConnectedAgents.Set(float64(len(reg.agents)))
		reg.mu.Unlock()

		// keep connection alive and update LastSeen; remove on close
		go func(id string, c *websocket.Conn) {
			defer func() {
				c.Close()
				reg.mu.Lock()
				// We don't necessarily delete from persistence here, just from memory if connection drops?
				// For now, let's keep the behavior consistent: delete from memory map to indicate offline.
				// But we keep in DB? Design doc said "Prune 'dead' agents after N days".
				// So we leave in DB, but maybe update last_seen?
				// The current in-memory map logic deletes it.
				// If we restart Hub, we load from DB. If Agent is offline, it won't reconnect.
				// So we probably want to keep it in DB.
				if agent, ok := reg.agents[id]; ok {
					// Mark as disconnected in memory?
					// The existing logic deletes it entirely.
					// Let's stick to existing logic for now, but update store if we want persistence.
					// Actually, if we delete from memory, we should probably keep in DB so it shows up as "offline" eventually?
					// For this implementation, let's just follow the existing memory pattern but don't delete from DB
					// so it survives restart if it reconnects quickly.
					// However, the `reg.agents` map is the source of truth for "online" agents for routing.
				delete(reg.agents, id)
				hubConnectedAgents.Set(float64(len(reg.agents)))
				reg.mu.Unlock()
			}()

			for {
				var msg map[string]any
				if err := c.ReadJSON(&msg); err != nil {
					return
				}

				// Update last seen
				reg.mu.Lock()
				if ag, ok := reg.agents[id]; ok {
					ag.LastSeen = time.Now().Unix()
					// We could update DB here but doing it on every message might be too much.
					// Let's do it periodically or on significant events.
					// For now, just memory.
				}
				reg.mu.Unlock()

				// If agent reports status for an app, publish it to the hub so waiting clients can be notified.
				if t, ok := msg["type"].(string); ok && t == "status" {
					if appName, ok := msg["app"].(string); ok {
						h.Publish(Message{Topic: "app:" + appName, Payload: msg, From: id})
					}
				}
			}
		}(ag.ID, conn)
	})

	// Notification websocket for splash clients: /ws/notify?app=NAME
	mux.HandleFunc("/ws/notify", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		app := q.Get("app")
		if app == "" {
			http.Error(w, "missing app", http.StatusBadRequest)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		sub, unsub := h.Subscribe("app:" + app)
		defer func() {
			unsub()
			c.Close()
		}()

		// forward any messages for the app to the websocket client
		for msg := range sub {
			_ = c.WriteJSON(msg.Payload)
		}
	})

	// POST /wake?app=NAME - manually trigger a wake command
	mux.HandleFunc("/wake", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		app := q.Get("app")
		if app == "" {
			http.Error(w, "missing app", http.StatusBadRequest)
			return
		}

		// find agent for app
		reg.mu.RLock()
		var target *Agent
		for _, a := range reg.agents {
			for _, ap := range a.Apps {
				if ap == app {
					target = a
					break
				}
			}
			if target != nil {
				break
			}
		}
		reg.mu.RUnlock()

		if target == nil {
			http.Error(w, "app not found", http.StatusNotFound)
			return
		}

		// send WAKE
		hubWakeRequestsTotal.Inc()
		reg.mu.RLock()
		conn := target.Conn
		reg.mu.RUnlock()
		wakeMsg := map[string]any{"type": "wake", "app": app}
		if conn != nil {
			_ = conn.WriteJSON(wakeMsg)
		}
		h.Publish(Message{Topic: "wake:" + app, Payload: wakeMsg, From: "hub"})

		w.WriteHeader(http.StatusAccepted)
	})

	// Core HTTP proxy: inspect Host header (subdomain) and proxy to registered agent
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hubRequestsTotal.Inc()
		host := r.Host
		// strip port if present
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}

		// assume first label is the app name: app.example.com
		parts := strings.Split(host, ".")
		if len(parts) == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		app := parts[0]

		// find an agent that advertises this app
		reg.mu.RLock()
		var target *Agent
		for _, a := range reg.agents {
			for _, ap := range a.Apps {
				if ap == app {
					target = a
					break
				}
			}
			if target != nil {
				break
			}
		}
		reg.mu.RUnlock()

		if target == nil {
			http.Error(w, "service not found", http.StatusNotFound)
			return
		}

		// prepare reverse proxy to agent address
		targetURL := &url.URL{Scheme: "http", Host: target.Addr}
		proxy := httputil.NewSingleHostReverseProxy(targetURL)

		// Intercept 502/503 from Agent (indicating App is down) to trigger Splash Screen
		proxy.ModifyResponse = func(resp *http.Response) error {
			if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusServiceUnavailable {
				return fmt.Errorf("upstream app unavailable")
			}
			return nil
		}

		// if proxying fails (e.g., agent container down), we trigger a wake and show splash
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			hubWakeRequestsTotal.Inc()
			// send WAKE to the agent if websocket connection is present
			reg.mu.RLock()
			conn := target.Conn
			reg.mu.RUnlock()
			wakeMsg := map[string]any{"type": "wake", "app": app}
			if conn != nil {
				_ = conn.WriteJSON(wakeMsg)
			}
			// also publish to local hub for any internal subscribers
			h.Publish(Message{Topic: "wake:" + app, Payload: wakeMsg, From: "hub"})

			// respond with splash HTML that listens for readiness and allows a manual wake request
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.WriteHeader(http.StatusOK)
			splash := `<!doctype html><html><head><meta charset="utf-8"><title>Waking ` + app + `</title></head><body><h1>Service: ` + app + `</h1><p>The machine hosting this app appears to be powered off or unreachable. Click "Wake" to attempt to wake the host; this page will automatically reload when the service reports ready.</p><button id="wake">Wake</button> <span id="status"></span><script>let ws=new WebSocket((location.protocol==='https:'?'wss':'ws')+'://'+location.host+'/ws/notify?app=` + app + `');ws.onmessage=e=>{try{let m=JSON.parse(e.data);if(m.state==='READY'){location.reload();}else{document.getElementById('status').textContent=JSON.stringify(m);}}catch(err){};};document.getElementById('wake').onclick=function(){var btn=this;btn.disabled=true;document.getElementById('status').textContent='Sending wake request...';fetch('/wake?app=` + app + `',{method:'POST'}).then(res=>{if(res.ok||res.status===202){document.getElementById('status').textContent='Wake request sent.'}else{document.getElementById('status').textContent='Wake request failed: '+res.status;btn.disabled=false}}).catch(e=>{document.getElementById('status').textContent='Error: '+e;btn.disabled=false})};</script></body></html>`
			_, _ = rw.Write([]byte(splash))
		}

		proxy.ServeHTTP(w, r)
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

	logger.Info("server listening", "addr", addr)
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down server")
			shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutCtx)
			return nil
		case err := <-serverErr:
			if err != nil {
				logger.Error("server error", "error", err)
				return err
			}
		case <-ticker.C:
			// heartbeat publish (non-blocking to subscribers)
			h.Publish(Message{Topic: "heartbeat", Payload: time.Now(), From: "server"})
		}
	}
}
