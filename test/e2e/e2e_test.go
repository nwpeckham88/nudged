package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestE2E(t *testing.T) {
	// 1. Setup: Start environment
	t.Log("Starting docker-compose environment...")
	cmd := exec.Command("docker", "compose", "-f", "../../docker-compose.yml", "up", "-d", "--build", "--force-recreate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to start docker-compose: %v", err)
	}
	defer func() {
		if os.Getenv("SKIP_TEARDOWN") != "" {
			return
		}
		t.Log("Tearing down environment...")
		_ = exec.Command("docker", "compose", "-f", "../../docker-compose.yml", "down").Run()
	}()

	// 2. Wait for Hub Health
	t.Log("Waiting for Hub to be healthy...")
	if err := waitForURL("http://localhost:8080/health", 30*time.Second); err != nil {
		t.Fatalf("Hub not healthy: %v", err)
	}

	// 3. Wait for Agent Registration
	t.Log("Waiting for Agent to register...")
	if err := waitForAgent("testsvc", 30*time.Second); err != nil {
		t.Fatalf("Agent registration failed: %v", err)
	}

	// 4. Test Proxy (Happy Path)
	t.Log("Testing proxy to running service...")
	content, err := fetchWithHost("http://localhost:8080/", "testsvc.localhost")
	if err != nil {
		t.Fatalf("Proxy request failed: %v", err)
	}
	if !strings.Contains(content, "Welcome to nginx!") {
		t.Errorf("Unexpected content from nginx: %s", content[:100])
	}

	// 5. Test Wake (Stop -> Request -> Wake -> Verify)
	t.Log("Testing Wake-on-Request...")
	
	// Stop the test service container
	// We need to find the container name. docker-compose usually names it nudged-testsvc-1 or similar.
	// But we can use docker compose stop
	t.Log("Stopping testsvc...")
	if err := exec.Command("docker", "compose", "-f", "../../docker-compose.yml", "stop", "testsvc").Run(); err != nil {
		t.Fatalf("Failed to stop testsvc: %v", err)
	}

	// Verify it's stopped (proxy should return splash screen or 503, currently splash screen with 200 OK)
	// The Hub code: proxy.ErrorHandler -> responds 200 OK with splash HTML
	t.Log("Verifying splash screen...")
	splashContent, err := fetchWithHost("http://localhost:8080/", "testsvc.localhost")
	if err != nil {
		t.Fatalf("Failed to fetch splash screen: %v", err)
	}
	if !strings.Contains(splashContent, "Waking testsvc") {
		limit := 100
		if len(splashContent) < limit {
			limit = len(splashContent)
		}
		t.Errorf("Expected splash screen, got: %s", splashContent[:limit])
	}

	// Trigger Wake (simulate button click)
	t.Log("Triggering wake...")
	resp, err := http.Post("http://localhost:8080/wake?app=testsvc", "text/plain", nil)
	if err != nil {
		t.Fatalf("Failed to send wake request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		t.Errorf("Wake request returned status %d", resp.StatusCode)
	}

	// Wait for service to come back
	t.Log("Waiting for service to return...")
	start := time.Now()
	recovered := false
	for time.Since(start) < 30*time.Second {
		content, err := fetchWithHost("http://localhost:8080/", "testsvc.localhost")
		if err == nil && strings.Contains(content, "Welcome to nginx!") {
			recovered = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !recovered {
		t.Fatal("Service did not recover after wake")
	}

	t.Log("E2E Test Passed!")
}

func waitForURL(url string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		resp, err := http.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func waitForAgent(appName string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		resp, err := http.Get("http://localhost:8080/agents")
		if err == nil && resp.StatusCode == http.StatusOK {
			var agents []map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&agents); err == nil {
				for _, a := range agents {
					if apps, ok := a["apps"].([]interface{}); ok {
						for _, app := range apps {
							if appStr, ok := app.(string); ok && appStr == appName {
								resp.Body.Close()
								return nil
							}
						}
					}
				}
			}
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent with app %s", appName)
}

func fetchWithHost(urlStr, host string) (string, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", err
	}
	req.Host = host

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
