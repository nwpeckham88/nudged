package server

import (
    "fmt"
    "time"

    "nudged/internal/hub"
)

// Start creates a hub instance and performs a tiny self-check.
func Start() error {
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

    return nil
}
