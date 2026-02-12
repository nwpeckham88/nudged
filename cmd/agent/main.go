package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "net/http"
    "net/url"
    "os"
    "strings"
    "time"

    "github.com/gorilla/websocket"
)

func env(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func extractPort(addr string, def string) string {
    // addr may be host:port or just port
    if idx := strings.LastIndex(addr, ":"); idx != -1 {
        return addr[idx+1:]
    }
    return def
}

func main() {
    // allow a simple --help flag
    hlp := flag.Bool("help", false, "show help")
    flag.Parse()
    if *hlp {
        flag.Usage()
        return
    }

    hubAddr := env("HUB_ADDR", "ws://hub:8080/ws/register")
    agentID := env("AGENT_ID", "agent-local")
    agentName := env("AGENT_NAME", "agent-local")
    agentAddr := env("AGENT_ADDR", "127.0.0.1:8081")
    apps := env("AGENT_APPS", "demo")

    port := extractPort(agentAddr, "8081")

    // start a small health endpoint for the agent on the configured port
    go func() {
        http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
            w.Header().Set("Content-Type", "application/json")
            _ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "status": "ok"})
        })
        listen := ":" + port
        log.Printf("agent health listening on %s", listen)
        if err := http.ListenAndServe(listen, nil); err != nil {
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
