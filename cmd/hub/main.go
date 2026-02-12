package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"nudged/pkg/server"
)

var version = "0.1.0"

func main() {
	cfg := flag.String("config", "", "path to config file")
	ver := flag.Bool("version", false, "print version")
	serve := flag.Bool("serve", false, "start server")
	flag.Parse()

	if *ver {
		fmt.Println(version)
		return
	}

	if *serve {
		// propagate signals to context for graceful shutdown
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		fmt.Printf("starting nudged server v%s\n", version)
		if err := server.Start(ctx, ":8080"); err != nil {
			fmt.Fprintf(os.Stderr, "server failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Printf("nudged v%s\n", version)
	if *cfg != "" {
		fmt.Printf("Using config: %s\n", *cfg)
	}

	os.Exit(0)
}
