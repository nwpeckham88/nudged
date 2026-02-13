package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nwpeckham88/nudged/internal/agent"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
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

	cfg := agent.Config{
		HubAddr: env("HUB_ADDR", "ws://localhost:8080/ws/register"),
		ID:      env("AGENT_ID", "agent-local"),
		Name:    env("AGENT_NAME", "agent-local"),
		Addr:    env("AGENT_ADDR", "127.0.0.1:8081"),
		Secret:  env("NUDGED_HUB_SECRET", ""),
		Mock:    env("AGENT_MOCK", "false") == "true",
	}

	a, err := agent.New(cfg)
	if err != nil {
		log.Fatalf("failed to create agent: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.Run(ctx); err != nil {
		log.Fatalf("agent stopped: %v", err)
	}
}
