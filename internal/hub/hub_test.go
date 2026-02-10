package hub

import (
    "testing"
    "time"
)

func TestPublishSubscribe(t *testing.T) {
    h := New()

    sub, unsub := h.Subscribe("topic1")
    defer unsub()

    go h.Publish(Message{Topic: "topic1", Payload: "hello", From: "test"})

    select {
    case msg := <-sub:
        if msg.Payload != "hello" {
            t.Fatalf("unexpected payload: %v", msg.Payload)
        }
    case <-time.After(500 * time.Millisecond):
        t.Fatal("timed out waiting for message")
    }
}
