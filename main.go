package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration.
type Config struct {
	SuccessURLPattern string
	SuccessSelector   string
	UserDataDir       string
	Headless          bool
	Timeout           time.Duration
}

// YamlConfig is an intermediate struct to parse YAML `timeout_seconds`.
type YamlConfig struct {
	SuccessURLPattern string `yaml:"success_url_pattern"`
	SuccessSelector   string `yaml:"success_selector"`
	UserDataDir       string `yaml:"user_data_dir"`
	Headless          bool   `yaml:"headless"`
	TimeoutSeconds    int    `yaml:"timeout_seconds"`
}

// loadConfig reads a YAML file from the given path and returns a Config struct.
func loadConfig(path string) (Config, error) {
	var cfg Config
	var yamlCfg YamlConfig

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &yamlCfg); err != nil {
		return cfg, fmt.Errorf("unmarshal yaml: %w", err)
	}

	cfg.SuccessURLPattern = yamlCfg.SuccessURLPattern
	cfg.SuccessSelector = yamlCfg.SuccessSelector
	absPath, err := filepath.Abs(yamlCfg.UserDataDir)
	if err != nil {
		return cfg, fmt.Errorf("could not get absolute path for UserDataDir: %w", err)
	}
	cfg.UserDataDir = absPath
	cfg.Headless = yamlCfg.Headless
	cfg.Timeout = time.Duration(yamlCfg.TimeoutSeconds) * time.Second

	return cfg, nil
}

func main() {
	rand.Seed(time.Now().UnixNano())
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]
	var configFile string
	var eraseData bool
	var queryParts []string

	// Handle flags and arguments
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--erase-data" {
			if command != "auth" {
				fmt.Fprintln(os.Stderr, "Error: --erase-data flag is only valid for the 'auth' command.")
				printUsage()
				os.Exit(1)
			}
			eraseData = true
			continue
		}

		if strings.HasSuffix(args[i], ".yml") || strings.HasSuffix(args[i], ".yaml") {
			configFile = args[i]
			continue
		}

		queryParts = append(queryParts, args[i])
	}

	if configFile == "" {
		configFile = "config.yml" // Default filename
	}

	cfg, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("Failed to load config from '%s': %v", configFile, err)
	}

	switch command {
	case "auth":
		fmt.Print("Please provide the authentication URL: ")
		var authURL string
		if _, err := fmt.Scanln(&authURL); err != nil {
			log.Fatalf("Failed to read URL: %v", err)
		}
		fmt.Println("Running authentication process...")
		if err := runAuth(cfg, eraseData, authURL); err != nil {
			log.Fatalf("Authentication failed: %v", err)
		}
	case "search":
		if len(queryParts) == 0 {
			log.Fatalf("Search command requires at least one argument.")
		}
		query := strings.Join(queryParts, " ")
		if err := runSearch(cfg, query); err != nil {
			log.Fatalf("Search failed: %v", err)
		}
	default:
		log.Fatalf("Unknown command: %s. Use 'auth' or 'search'.", command)
	}
}

// simulateHumanBehavior attempts to move the mouse randomly across the page
// to mimic human interaction.
func simulateHumanBehavior(page playwright.Page) error {
	size := page.ViewportSize()
	if size == nil {
		return fmt.Errorf("viewport size is nil, cannot simulate mouse movements")
	}

	// Number of random movements
	numMovements := 5 + rand.Intn(5) // Between 5 and 9 movements

	for i := 0; i < numMovements; i++ {
		// Generate random target coordinates within the viewport
		targetX := rand.Intn(size.Width)
		targetY := rand.Intn(size.Height)

		// Move mouse to target with a random step duration (e.g., 50-200ms)
		err := page.Mouse().Move(float64(targetX), float64(targetY), playwright.MouseMoveOptions{
			Steps: playwright.Int(rand.Intn(10) + 5), // 5 to 14 steps for smoother movement
		})
		if err != nil {
			return fmt.Errorf("could not move mouse: %w", err)
		}

		// Random delay after each movement (e.g., 100-300ms)
		time.Sleep(time.Duration(rand.Intn(200)+100) * time.Millisecond)
	}

	return nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: go run main.go <command> [options] [config.yml]")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  auth         Run the interactive authentication process.")
	fmt.Fprintln(os.Stderr, "  search       Search for items on Wolt.")
	fmt.Fprintln(os.Stderr, "\nGlobal Options:")
	fmt.Fprintln(os.Stderr, "  --help, -h   Show this help message and exit.")
	fmt.Fprintln(os.Stderr, "\nOptions for 'auth' command:")
	fmt.Fprintln(os.Stderr, "  --erase-data Force deletion of existing session data before authenticating.")
	fmt.Fprintln(os.Stderr, "\nArguments:")
	fmt.Fprintln(os.Stderr, "  [config.yml] (Optional) Path to the config file. Defaults to 'config.yml'.")
}

func runAuth(cfg Config, eraseData bool, authURL string) error {
	if eraseData {
		fmt.Printf("Erasing session data from '%s'...\n", cfg.UserDataDir)
		if err := os.RemoveAll(cfg.UserDataDir); err != nil {
			return fmt.Errorf("failed to erase user data directory: %w", err)
		}
	}

	fmt.Println("Starting Playwright...")
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	fmt.Println("Playwright started.")

	if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
		return fmt.Errorf("could not create user data directory: %w", err)
	}
	fmt.Println("User data directory ensured:", cfg.UserDataDir)

	fmt.Println("Launching persistent browser context...")
	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(false), // Always interactive for auth
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return fmt.Errorf("could not launch persistent context: %w", err)
	}
	fmt.Println("Browser context launched.")

	script := `
		Object.defineProperty(navigator, 'webdriver', { get: () => false });
		Object.defineProperty(navigator, 'plugins', {
			get: () => [
				{ name: 'PDF Viewer', filename: 'internal-pdf-viewer', description: 'Portable Document Format' },
			],
		});
		const originalQuery = navigator.permissions.query;
		navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications'
				? Promise.resolve({ state: 'prompt' })
				: originalQuery(parameters)
		);
	`
	fmt.Println("Adding init script to hide automation indicators...")
	err = ctx.AddInitScript(playwright.Script{Content: &script})
	if err != nil {
		return fmt.Errorf("could not add init script: %w", err)
	}
	fmt.Println("Init script added.")

	fmt.Println("Creating new page...")
	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("could not create new page: %w", err)
	}
	fmt.Println("Page created.")

	fmt.Println("Setting viewport size to 1440x810...")
	if err := page.SetViewportSize(1440, 810); err != nil {
		return fmt.Errorf("could not set viewport size: %w", err)
	}
	fmt.Println("Viewport size set.")

	fmt.Println("Setting up header removal...")
	if err := setupHeaderRemoval(page); err != nil {
		return fmt.Errorf("could not set up header removal: %w", err)
	}
	fmt.Println("Header removal setup complete.")

	fmt.Println("Navigating to:", authURL)
	if _, err = page.Goto(authURL); err != nil {
		return fmt.Errorf("could not go to auth URL: %w", err)
	}
	fmt.Println("Navigation initiated. Waiting for network to be idle.")

	// Wait for the page to be fully loaded (including network idle)
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle, Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds()))}); err != nil {
		return fmt.Errorf("failed to wait for page network idle: %w", err)
	}
	fmt.Println("Page network is idle.")

	// Simulate human-like mouse movements
	if err := simulateHumanBehavior(page); err != nil {
		log.Printf("Warning: Could not simulate human behavior: %v", err)
	}
	fmt.Println("Human behavior simulation complete. Waiting for page network to be idle again.")

	// Wait for the page to be fully loaded (including network idle) after potential redirects/JS execution
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle, Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds()))}); err != nil {
		return fmt.Errorf("failed to wait for page network idle after human simulation: %w", err)
	}
	fmt.Println("Page network is idle again.")

	// Attempt to click the "Use only necessary" button first, if it exists
	fmt.Println("Attempting to find and click 'Use only necessary' button...")
	useNecessaryButtonLocator := page.Locator("text=Use only necessary").Nth(0)
	if err := useNecessaryButtonLocator.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000), // Shorter timeout for this button
	}); err == nil {
		fmt.Println("'Use only necessary' button found. Clicking...")
		if err := useNecessaryButtonLocator.Click(); err != nil {
			log.Printf("Warning: Could not click 'Use only necessary' button: %v", err)
		} else {
			fmt.Println("'Use only necessary' button clicked. Waiting for page to settle after click.")
			if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle, Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds()))}); err != nil {
				log.Printf("Warning: Failed to wait for network idle after 'Use only necessary' click: %v", err)
			}
			fmt.Println("Page settled after 'Use only necessary' click.")
		}
	} else {
		fmt.Println("'Use only necessary' button not found or not visible within timeout.")
	}

	// Click the decline button first, if it exists
	fmt.Println("Attempting to find and click decline button with data-test-id=\"decline-button\"...")
	declineButtonSelector := "[data-test-id=\"decline-button\"]"
	declineButtonLocator := page.Locator(declineButtonSelector).Nth(0)
	if err := declineButtonLocator.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000), // Shorter timeout for decline button, as it might not always exist
	}); err == nil {
		fmt.Println("Decline button found. Clicking...")
		if err := declineButtonLocator.Click(); err != nil {
			log.Printf("Warning: Could not click decline button '%s': %v", declineButtonSelector, err)
		} else {
			fmt.Println("Decline button clicked. Waiting for page to settle after click.")
			// Wait for page to settle after click
			if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle, Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds()))}); err != nil {
				log.Printf("Warning: Failed to wait for network idle after decline button click: %v", err)
			}
			fmt.Println("Page settled after decline button click.")
		}
	} else {
		fmt.Println("Decline button not found or not visible within timeout (this is often expected).")
	}

	// Find the confirm button and click it
	fmt.Println("Waiting for confirm button with data-test-id=\"magic-login-landing.confirm\"...")
	confirmButtonSelector := "[data-test-id=\"magic-login-landing.confirm\"]"
	confirmButton, err := page.WaitForSelector(confirmButtonSelector, playwright.PageWaitForSelectorOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	})
	if err != nil {
		return fmt.Errorf("could not find or wait for confirm button '%s': %w", confirmButtonSelector, err)
	}
	fmt.Println("Confirm button found. Clicking...")
	if err := confirmButton.Click(); err != nil {
		return fmt.Errorf("could not click confirm button '%s': %w", confirmButtonSelector, err)
	}
	fmt.Println("Confirm button clicked. Waiting for navigation to 'https://wolt.com/en/discovery'.")

	// Wait for the page to reload into the success URL
	successURL := "https://wolt.com/en/discovery"
	if err := page.WaitForURL(successURL, playwright.PageWaitForURLOptions{
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("failed to navigate to '%s': %w", successURL, err)
	}
	fmt.Println("Successfully navigated to discovery page.")

	fmt.Println("Login successful. Session persisted in:", cfg.UserDataDir)

	fmt.Println("Waiting 5 seconds before closing the browser...")
	time.Sleep(5 * time.Second)
	fmt.Println("Closing browser.")

	if err := ctx.Close(); err != nil {
		log.Printf("Warning: Could not close browser context: %v", err)
	}
	// Stop Playwright after the context is closed and saved
	if err := pw.Stop(); err != nil {
		return fmt.Errorf("could not stop playwright: %w", err)
	}
	return nil
}

func runAutomation(cfg Config) error {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	// Playwright started.

	if _, err := os.Stat(cfg.UserDataDir); os.IsNotExist(err) {
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("no saved session found in '%s'. Please run the 'auth' command first", cfg.UserDataDir)
	}

	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(cfg.Headless),
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		if err := pw.Stop(); err != nil {
			log.Printf("Warning: Could not stop playwright: %v", err)
		}
		return fmt.Errorf("could not launch persistent context: %w", err)
	}

	// Inject script to hide automation indicators.
	script := `
		Object.defineProperty(navigator, 'webdriver', { get: () => false });
		Object.defineProperty(navigator, 'plugins', {
			get: () => [
				{ name: 'PDF Viewer', filename: 'internal-pdf-viewer', description: 'Portable Document Format' },
			],
		});
		const originalQuery = navigator.permissions.query;
		navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications'
				? Promise.resolve({ state: 'prompt' })
				: originalQuery(parameters)
		);
	`
	err = ctx.AddInitScript(playwright.Script{Content: &script})
	if err != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("could not add init script: %w", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("could not create new page: %w", err)
	}
	fmt.Println("Setting viewport size to 1440x810...")
	if err := page.SetViewportSize(1440, 810); err != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("could not set viewport size: %w", err)
	}
	if err := setupHeaderRemoval(page); err != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("could not set up header removal: %w", err)
	}

	if cfg.SuccessURLPattern == "" {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("SuccessURLPattern must be configured for the 'run' command")
	}

	fmt.Println("Navigating to:", cfg.SuccessURLPattern)
	if _, err = page.Goto(cfg.SuccessURLPattern); err != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("could not go to success URL pattern: %w", err)
	}

	// Simulate human-like mouse movements
	if err := simulateHumanBehavior(page); err != nil {
		log.Printf("Warning: Could not simulate human behavior: %v", err)
	}

	// Quick auth check with a short timeout
	fmt.Println("Verifying session...")
	checkCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := waitAuthorizedWithContext(checkCtx, page, cfg); err != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("session appears to be expired or invalid. Please run the 'auth' command again: %w", err)
	}

	fmt.Println("Session is valid. Automation steps would run here.")
	// ... do automation steps here ...
	// For demonstration, let's take a screenshot.
	screenshotPath := "automation_screenshot.png"
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(screenshotPath),
	}); err != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
		if err := pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
		return fmt.Errorf("failed to take screenshot: %w", err)
	}
	fmt.Println("Took a screenshot:", screenshotPath)

	if err := ctx.Close(); err != nil {
		log.Printf("Warning: Could not close browser context: %v", err)
	}
	if err := pw.Stop(); err != nil {
		return fmt.Errorf("could not stop playwright: %w", err)
	}

	return nil
}

func runSearch(cfg Config, query string) error {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()

	if _, err := os.Stat(cfg.UserDataDir); os.IsNotExist(err) {
		return fmt.Errorf("no saved session found in '%s'. Please run the 'auth' command first", cfg.UserDataDir)
	}

	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(cfg.Headless),
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return fmt.Errorf("could not launch persistent context: %w", err)
	}
	defer ctx.Close()

	// Inject script to hide automation indicators.
	script := `
		Object.defineProperty(navigator, 'webdriver', { get: () => false });
		Object.defineProperty(navigator, 'plugins', {
			get: () => [
				{ name: 'PDF Viewer', filename: 'internal-pdf-viewer', description: 'Portable Document Format' },
			],
		});
		const originalQuery = navigator.permissions.query;
		navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications'
				? Promise.resolve({ state: 'prompt' })
				: originalQuery(parameters)
		);
	`
	if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("could not add init script: %w", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("could not create new page: %w", err)
	}

	if err := page.SetViewportSize(1440, 810); err != nil {
		return fmt.Errorf("could not set viewport size: %w", err)
	}

	if err := setupHeaderRemoval(page); err != nil {
		return fmt.Errorf("could not set up header removal: %w", err)
	}

	searchURL := fmt.Sprintf("https://wolt.com/en/search?q=%s&target=items&filters=delivers_now%%3Ddelivers_now_toggle", url.QueryEscape(query))

	fmt.Printf("Searching for: %s\n", query)

	var body []byte

	// Use a channel to capture the right response to avoid deadlock and handle multiple requests
	type searchRes struct {
		body []byte
		err  error
	}
	resChan := make(chan searchRes, 1)

	page.OnResponse(func(res playwright.Response) {
		if strings.Contains(res.URL(), "https://restaurant-api.wolt.com/v1/pages/search") {
			if res.Request().Method() == "OPTIONS" {
				return
			}
			// Process in a goroutine to avoid deadlocking the event loop
			go func() {
				b, err := res.Body()
				select {
				case resChan <- searchRes{body: b, err: err}:
				default:
					// Already captured a response
				}
			}()
		}
	})

	if _, err = page.Goto(searchURL); err != nil {
		return fmt.Errorf("could not go to search URL: %w", err)
	}

	select {
	case res := <-resChan:
		if res.err != nil {
			return fmt.Errorf("could not read response body: %w", res.err)
		}
		body = res.body
	case <-time.After(cfg.Timeout):
		return fmt.Errorf("timed out waiting for API response from https://restaurant-api.wolt.com/v1/pages/search")
	}

	var responseJSON interface{}
	if err := json.Unmarshal(body, &responseJSON); err != nil {
		return fmt.Errorf("could not unmarshal response JSON: %w. Body length: %d", err, len(body))
	}

	// Extract count of sections.items
	count := 0
	if resMap, ok := responseJSON.(map[string]interface{}); ok {
		if sections, ok := resMap["sections"].([]interface{}); ok {
			for _, s := range sections {
				if section, ok := s.(map[string]interface{}); ok {
					if items, ok := section["items"].([]interface{}); ok {
						count += len(items)
					}
				}
			}
		}
	}

	result := map[string]interface{}{
		"keyword": query,
		"count":   count,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))

	return nil
}

// setupHeaderRemoval intercepts network requests and removes specified headers.
func setupHeaderRemoval(page playwright.Page) error {
	return page.Route("**/*", func(route playwright.Route) {
		headers := route.Request().Headers()
		delete(headers, "sec-ch-ua")
		delete(headers, "sec-ch-ua-mobile")
		delete(headers, "sec-ch-ua-platform")
		if err := route.Continue(playwright.RouteContinueOptions{
			Headers: headers,
		}); err != nil {
			log.Printf("could not continue route: %v", err)
		}
	})
}

func waitAuthorized(page playwright.Page, cfg Config) error {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute // Default from user plan
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return waitAuthorizedWithContext(ctx, page, cfg)
}

func waitAuthorizedWithContext(ctx context.Context, page playwright.Page, cfg Config) error {
	if cfg.SuccessURLPattern == "" && cfg.SuccessSelector == "" {
		return fmt.Errorf("no success_url_pattern or success_selector configured")
	}

	errCh := make(chan error, 2)
	var checks int

	if cfg.SuccessURLPattern != "" {
		checks++
		go func() {
			err := page.WaitForURL(cfg.SuccessURLPattern, playwright.PageWaitForURLOptions{
				Timeout: playwright.Float(0), // Context handles timeout
			})
			errCh <- err
		}()
	}

	if cfg.SuccessSelector != "" {
		checks++
		go func() {
			_, err := page.WaitForSelector(cfg.SuccessSelector, playwright.PageWaitForSelectorOptions{
				State:   playwright.WaitForSelectorStateAttached,
				Timeout: playwright.Float(0), // Context handles timeout
			})
			errCh <- err
		}()
	}

	// If only one check is configured, we can return its result directly.
	if checks == 1 {
		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for authorization signal: %w", ctx.Err())
		}
	}

	// If two checks are configured, we succeed if either one succeeds.
	for i := 0; i < checks; i++ {
		select {
		case err := <-errCh:
			if err == nil {
				// One of the conditions was met successfully.
				return nil
			}
			// One of the checks failed, but the other might still succeed.
			log.Printf("A login check failed: %v. Waiting for other checks.", err)
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for authorization signal: %w", ctx.Err())
		}
	}

	return fmt.Errorf("all authorization checks failed")
}
