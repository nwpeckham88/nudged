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

	// Run Subtests
	t.Run("Nginx_HappyPath", testNginxHappyPath)
	t.Run("Nginx_Wake", testNginxWake)
	t.Run("SimService_Lifecycle", testSimServiceLifecycle)
}

func testNginxHappyPath(t *testing.T) {
	// 3. Wait for Agent Registration
	t.Log("Waiting for Agent to register testsvc...")
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
}

func testNginxWake(t *testing.T) {
	// 5. Test Wake (Stop -> Request -> Wake -> Verify)
	t.Log("Testing Wake-on-Request...")
	
	// Stop the test service container
	t.Log("Stopping testsvc...")
	if err := exec.Command("docker", "compose", "-f", "../../docker-compose.yml", "stop", "testsvc").Run(); err != nil {
		t.Fatalf("Failed to stop testsvc: %v", err)
	}

	// Verify it's stopped (proxy should return splash screen or 503)
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
}

func testSimServiceLifecycle(t *testing.T) {
	// Wait for registration
	if err := waitForAgent("simsvc", 30*time.Second); err != nil {
		t.Fatalf("Agent registration failed for simsvc: %v", err)
	}

	// Stop simsvc to test Wake + Slow Startup
	t.Log("Stopping simsvc...")
	if err := exec.Command("docker", "compose", "-f", "../../docker-compose.yml", "stop", "simsvc").Run(); err != nil {
		t.Fatalf("Failed to stop simsvc: %v", err)
	}

	// Verify Splash
	t.Log("Verifying splash screen for simsvc...")
	splashContent, err := fetchWithHost("http://localhost:8080/", "simsvc.localhost")
	if err != nil {
		t.Fatalf("Failed to fetch splash: %v", err)
	}
	if !strings.Contains(splashContent, "Waking simsvc") {
		t.Errorf("Expected splash screen, got: %s", splashContent)
	}

	// Trigger Wake
	t.Log("Triggering wake for simsvc...")
	resp, err := http.Post("http://localhost:8080/wake?app=simsvc", "text/plain", nil)
	if err != nil {
		t.Fatalf("Failed to wake simsvc: %v", err)
	}
	resp.Body.Close()

	// Poll for readiness (should take at least 5s due to STARTUP_DELAY)
	t.Log("Waiting for simsvc to start (expect >5s delay)...")
	start := time.Now()
	
	ready := false
	for time.Since(start) < 20*time.Second {
		content, err := fetchWithHost("http://localhost:8080/", "simsvc.localhost")
		if err == nil && strings.Contains(content, "Sim Service Ready") {
			ready = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !ready {
		t.Fatal("simsvc did not become ready within timeout")
	}

	duration := time.Since(start)
	t.Logf("simsvc became ready in %v", duration)
	if duration < 5*time.Second {
		t.Errorf("simsvc started too fast (%v), expected >5s delay", duration)
	}

	// Test Idle Timeout
	// nudged.timeout is set to 30s in docker-compose.
	t.Log("Waiting for idle timeout (30s)...")
	// We wait 35s to be sure
	time.Sleep(35 * time.Second)

	// Verify it's stopped.
	t.Log("Verifying simsvc is stopped...")
	splashContent, err = fetchWithHost("http://localhost:8080/", "simsvc.localhost")
	if err != nil {
		t.Fatalf("Failed to fetch after idle: %v", err)
	}
	if !strings.Contains(splashContent, "Waking simsvc") {
		if strings.Contains(splashContent, "Sim Service Ready") {
			t.Error("simsvc is still running after idle timeout")
		} else {
			t.Errorf("Expected splash screen, got: %s", splashContent)
		}
	} else {
		t.Log("Idle timeout verified: Service stopped.")
	}
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
