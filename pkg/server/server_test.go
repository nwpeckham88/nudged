package server

import "testing"

func TestStart(t *testing.T) {
    if err := Start(); err != nil {
        t.Fatalf("Start() failed: %v", err)
    }
}
