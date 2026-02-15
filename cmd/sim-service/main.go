package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}

	startupDelay := os.Getenv("STARTUP_DELAY")
	if startupDelay != "" {
		d, err := time.ParseDuration(startupDelay)
		if err != nil {
			log.Printf("Invalid STARTUP_DELAY: %v", err)
		} else {
			log.Printf("Simulating slow startup... sleeping for %v", d)
			time.Sleep(d)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Sim Service Ready! (Startup Delay: %s)", startupDelay)
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("Simon Service listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down...")

	shutdownDelay := os.Getenv("SHUTDOWN_DELAY")
	if shutdownDelay != "" {
		d, err := time.ParseDuration(shutdownDelay)
		if err == nil {
			log.Printf("Simulating slow shutdown... sleeping for %v", d)
			time.Sleep(d)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
	log.Println("Bye!")
}
