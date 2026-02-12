package hub

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	// Reserve a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- Start(ctx, addr)
	}()

	// give server a moment to start
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %v", resp.StatusCode)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() failed: %v", err)
		}
	case <-time.After(800 * time.Millisecond):
		t.Fatal("Start did not return")
	}
}
