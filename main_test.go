package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	content := `
success_url_pattern: "https://example.com/dashboard"
success_selector: "#user-avatar"
user_data_dir: "./test_profile"
headless: true
timeout_seconds: 30
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp config file: %v", err)
	}

	// Call loadConfig
	cfg, err := loadConfig(configFile)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Assert the results
	if cfg.SuccessURLPattern != "https://example.com/dashboard" {
		t.Errorf("expected SuccessURLPattern to be 'https://example.com/dashboard', got '%s'", cfg.SuccessURLPattern)
	}
	if cfg.SuccessSelector != "#user-avatar" {
		t.Errorf("expected SuccessSelector to be '#user-avatar', got '%s'", cfg.SuccessSelector)
	}
	if !filepath.IsAbs(cfg.UserDataDir) {
		t.Errorf("expected UserDataDir to be an absolute path, got '%s'", cfg.UserDataDir)
	}
	if filepath.Base(cfg.UserDataDir) != "test_profile" {
		t.Errorf("expected UserDataDir to end with 'test_profile', got '%s'", cfg.UserDataDir)
	}
	if !cfg.Headless {
		t.Errorf("expected Headless to be true, got false")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout to be 30s, got %v", cfg.Timeout)
	}
}

func TestRunAutomation_Success(t *testing.T) {
	// We need to install playwright browsers for the test
	if err := playwright.Install(&playwright.RunOptions{}); err != nil {
		t.Fatalf("could not install playwright: %v", err)
	}

	// Create a fake server that immediately serves the "logged in" page
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><div data-test-id="user-avatar"></div></body></html>`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "profile")
	// Create the UserDataDir so the os.Stat check passes
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		t.Fatalf("Failed to create temp user data dir: %v", err)
	}

	cfg := Config{
		SuccessURLPattern: server.URL,
		SuccessSelector:   "[data-test-id='user-avatar']",
		UserDataDir:       userDataDir,
		Headless:          true,
		Timeout:           15 * time.Second, // Short timeout for test
	}

	err := runAutomation(cfg)
	if err != nil {
		t.Errorf("runAutomation failed unexpectedly: %v", err)
	}
}

func TestRunAutomation_Failure_NoProfile(t *testing.T) {
	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "non_existent_profile")

	cfg := Config{
		SuccessURLPattern: "http://localhost:1234", // Placeholder, URL doesn't matter here
		UserDataDir:       userDataDir,
		Headless:          true,
		Timeout:           5 * time.Second,
	}

	err := runAutomation(cfg)
	if err == nil {
		t.Errorf("runAutomation was expected to fail but did not")
	}

	expectedError := "no saved session found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("expected error to contain '%s', but got '%s'", expectedError, err.Error())
	}
}
