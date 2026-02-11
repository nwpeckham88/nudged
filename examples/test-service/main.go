package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "math/rand"
    "net/http"
    "strconv"
    "sync/atomic"
    "time"
)

var ready int32

func main() {
    addr := flag.String("addr", ":8082", "listen address")
    flakyRate := flag.Float64("flaky", 0.0, "fraction of requests that fail (0..1)")
    startupDelay := flag.Int("startup", 0, "milliseconds to delay before becoming ready")
    flag.Parse()

    // optional startup delay to simulate slow start
    if *startupDelay > 0 {
        log.Printf("startup delay %dms", *startupDelay)
        time.Sleep(time.Duration(*startupDelay) * time.Millisecond)
    }
    atomic.StoreInt32(&ready, 1)

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if atomic.LoadInt32(&ready) == 0 {
            http.Error(w, "service starting", http.StatusServiceUnavailable)
            return
        }
        if rand.Float64() < *flakyRate {
            http.Error(w, "flaky error", http.StatusInternalServerError)
            return
        }
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "hello from test-service\n")
    })

    http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
        q := r.URL.Query()
        ms := 1000
        if s := q.Get("delay"); s != "" {
            if v, err := strconv.Atoi(s); err == nil {
                ms = v
            }
        }
        time.Sleep(time.Duration(ms) * time.Millisecond)
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "slept %dms\n", ms)
    })

    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        if atomic.LoadInt32(&ready) == 0 {
            w.WriteHeader(http.StatusServiceUnavailable)
            _ = json.NewEncoder(w).Encode(map[string]string{"status": "starting"})
            return
        }
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    })

    http.HandleFunc("/toggle-ready", func(w http.ResponseWriter, r *http.Request) {
        // POST to toggle readiness; GET returns current state
        if r.Method == http.MethodPost {
            v := r.URL.Query().Get("v")
            if v == "0" || v == "false" {
                atomic.StoreInt32(&ready, 0)
            } else if v == "1" || v == "true" {
                atomic.StoreInt32(&ready, 1)
            }
            w.WriteHeader(http.StatusOK)
            return
        }
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "%d", atomic.LoadInt32(&ready))
    })

    log.Printf("test-service listening on %s (flaky=%.2f)", *addr, *flakyRate)
    if err := http.ListenAndServe(*addr, nil); err != nil {
        log.Fatalf("listen: %v", err)
    }
}
