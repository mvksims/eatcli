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
	"strconv"
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

const (
	defaultBasketCaptureURL = "https://wolt.com/en/discovery"
	basketAPIURL            = "https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets"
)

func resolveUserDataDir(rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", fmt.Errorf("user_data_dir must be set and cannot be empty")
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("could not get absolute path for user_data_dir: %w", err)
	}
	cleaned := filepath.Clean(absPath)
	if filepath.Dir(cleaned) == cleaned {
		return "", fmt.Errorf("user_data_dir cannot be a filesystem root path: %s", cleaned)
	}

	return cleaned, nil
}

func validateEraseUserDataDir(path string) error {
	cleaned := filepath.Clean(path)
	if strings.TrimSpace(cleaned) == "" {
		return fmt.Errorf("refusing to erase empty user data directory path")
	}
	if filepath.Dir(cleaned) == cleaned {
		return fmt.Errorf("refusing to erase filesystem root path: %s", cleaned)
	}

	cwd, err := os.Getwd()
	if err == nil && filepath.Clean(cwd) == cleaned {
		return fmt.Errorf("refusing to erase current working directory: %s", cleaned)
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && filepath.Clean(homeDir) == cleaned {
		return fmt.Errorf("refusing to erase home directory: %s", cleaned)
	}

	return nil
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
	resolvedUserDataDir, err := resolveUserDataDir(yamlCfg.UserDataDir)
	if err != nil {
		return cfg, err
	}
	cfg.UserDataDir = resolvedUserDataDir
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
		fmt.Print("Go to wolt.com and try to login with your email address. Then copy the sign-in URL from the received email into here: ")
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
	case "basket":
		if len(queryParts) == 0 {
			if err := runBasket(cfg); err != nil {
				log.Fatalf("Basket failed: %v", err)
			}
			break
		}
		switch strings.ToLower(queryParts[0]) {
		case "add":
			if len(queryParts) != 3 {
				log.Fatalf("Basket add requires exactly 2 arguments: <venue_slug> <item_id>.")
			}
			venueSlug := queryParts[1]
			itemID := queryParts[2]
			if err := runBasketAdd(cfg, venueSlug, itemID); err != nil {
				log.Fatalf("Basket add failed: %v", err)
			}
		case "remove":
			if len(queryParts) != 3 {
				log.Fatalf("Basket remove requires exactly 2 arguments: <venue_slug> <item_id>.")
			}
			venueSlug := queryParts[1]
			itemID := queryParts[2]
			if err := runBasketRemove(cfg, venueSlug, itemID); err != nil {
				log.Fatalf("Basket remove failed: %v", err)
			}
		default:
			log.Fatalf("Unknown basket subcommand: %s. Use 'basket', 'basket add <venue_slug> <item_id>' or 'basket remove <venue_slug> <item_id>'.", queryParts[0])
		}
	case "checkout":
		if len(queryParts) != 1 {
			log.Fatalf("Checkout command requires exactly 1 argument: <venue_slug>.")
		}
		venueSlug := queryParts[0]
		if err := runCheckout(cfg, venueSlug); err != nil {
			log.Fatalf("Checkout failed: %v", err)
		}
	default:
		log.Fatalf("Unknown command: %s. Use 'auth', 'search', 'basket', or 'checkout'.", command)
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
	fmt.Fprintln(os.Stderr, "  auth         Sign in once to save your shopping session.")
	fmt.Fprintln(os.Stderr, "  search       Find products and return structured product details.")
	fmt.Fprintln(os.Stderr, "  basket       View current basket, add items, or remove items.")
	fmt.Fprintln(os.Stderr, "  checkout     Attempt order placement and report checkout error details if shown.")
	fmt.Fprintln(os.Stderr, "\nGlobal Options:")
	fmt.Fprintln(os.Stderr, "  --help, -h   Show this help message and exit.")
	fmt.Fprintln(os.Stderr, "\nOptions for 'auth' command:")
	fmt.Fprintln(os.Stderr, "  --erase-data Force deletion of existing session data before authenticating.")
	fmt.Fprintln(os.Stderr, "\nArguments:")
	fmt.Fprintln(os.Stderr, "  [config.yml] (Optional) Path to the config file. Defaults to 'config.yml'.")
	fmt.Fprintln(os.Stderr, "  basket Returns current basket JSON.")
	fmt.Fprintln(os.Stderr, "  basket add <venue_slug> <item_id> Increases item quantity and prints updated basket JSON.")
	fmt.Fprintln(os.Stderr, "  basket remove <venue_slug> <item_id> Removes the item from basket and prints updated basket JSON.")
	fmt.Fprintln(os.Stderr, "  checkout <venue_slug> Attempts order placement and reports checkout errors when present.")
}

func runAuth(cfg Config, eraseData bool, authURL string) error {
	if eraseData {
		if err := validateEraseUserDataDir(cfg.UserDataDir); err != nil {
			return err
		}
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

	products := extractSearchProducts(responseJSON)

	result := map[string]interface{}{
		"keyword":  query,
		"count":    len(products),
		"products": products,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))

	return nil
}

type basketResponseEvent struct {
	body   []byte
	err    error
	url    string
	status int
	at     time.Time
}

type basketResponseData struct {
	URL    string
	Status int
	JSON   interface{}
}

type BasketOutput struct {
	ID        string             `json:"id"`
	Total     interface{}        `json:"total"`
	VenueSlug string             `json:"venue_slug"`
	Items     []BasketItemOutput `json:"items"`
}

type BasketItemOutput struct {
	ID          string      `json:"id"`
	Count       int         `json:"count"`
	Total       interface{} `json:"total"`
	ImageURL    string      `json:"image_url"`
	Name        string      `json:"name"`
	IsAvailable bool        `json:"is_available"`
	Price       interface{} `json:"price"`
}

func runBasket(cfg Config) error {
	fmt.Println("Loading current basket...")

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

	resChan := attachBasketResponseCapture(page)
	requestStartedAt := time.Now()
	targetURL := defaultBasketCaptureURL
	if cfg.SuccessURLPattern != "" {
		targetURL = cfg.SuccessURLPattern
	}

	if _, err := page.Goto(targetURL); err != nil {
		return fmt.Errorf("could not go to basket page: %w", err)
	}

	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("basket page did not fully load: %w", err)
	}

	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, requestStartedAt)
	if err != nil {
		return err
	}

	baskets := extractBasketOutputs(basketRes.JSON)

	result := map[string]interface{}{
		"request": map[string]interface{}{
			"url":    basketRes.URL,
			"status": basketRes.Status,
		},
		"count":   len(baskets),
		"baskets": baskets,
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))
	return nil
}

func runBasketAdd(cfg Config, venueSlug, itemID string) error {
	basketAddDebugf("before: start add flow (venue_slug=%s item_id=%s)", venueSlug, itemID)
	basketAddDebugf("before: pre-check basket contents for venue/item")
	itemAlreadyInVenueBasket, err := isBasketItemPresentForVenue(cfg, venueSlug, itemID)
	if err != nil {
		basketAddDebugf("after: basket pre-check failed: %v", err)
		return err
	}
	basketAddDebugf("after: basket pre-check completed (present=%t)", itemAlreadyInVenueBasket)

	if itemAlreadyInVenueBasket {
		basketAddDebugf("before: route to checkout increment flow")
		if err := runBasketAddFromCheckout(cfg, venueSlug, itemID); err != nil {
			basketAddDebugf("after: checkout increment flow failed: %v", err)
			return err
		}
		basketAddDebugf("after: checkout increment flow completed")
		return nil
	}

	basketAddDebugf("before: route to direct item-page add flow")
	if err := runBasketAddFromProductDetail(cfg, venueSlug, itemID); err != nil {
		basketAddDebugf("after: direct item-page add flow failed: %v", err)
		return err
	}
	basketAddDebugf("after: direct item-page add flow completed")
	return nil
}

func runBasketAddFromProductDetail(cfg Config, venueSlug, itemID string) error {
	targetURL := buildBasketAddURL(venueSlug, itemID)
	basketAddDebugf("before: prepare product detail URL")
	fmt.Printf("Opening basket add page: %s\n", targetURL)
	basketAddDebugf("after: product detail URL ready")

	basketAddDebugf("before: start playwright for product detail add flow")
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()
	basketAddDebugf("after: playwright started for product detail add flow")

	basketAddDebugf("before: ensure user data dir exists (%s)", cfg.UserDataDir)
	if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
		return fmt.Errorf("could not create user data directory: %w", err)
	}
	basketAddDebugf("after: user data dir ready")

	basketAddDebugf("before: launch persistent browser context")
	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(false), // Always interactive for basket actions
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return fmt.Errorf("could not launch persistent context: %w", err)
	}
	defer ctx.Close()
	basketAddDebugf("after: persistent browser context launched")

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
	basketAddDebugf("before: add anti-detection init script")
	if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("could not add init script: %w", err)
	}
	basketAddDebugf("after: anti-detection init script added")

	basketAddDebugf("before: create new page")
	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("could not create new page: %w", err)
	}
	basketAddDebugf("after: new page created")

	basketAddDebugf("before: set viewport")
	if err := page.SetViewportSize(1440, 810); err != nil {
		return fmt.Errorf("could not set viewport size: %w", err)
	}
	basketAddDebugf("after: viewport set to 1440x810")

	basketAddDebugf("before: set up header removal")
	if err := setupHeaderRemoval(page); err != nil {
		return fmt.Errorf("could not set up header removal: %w", err)
	}
	basketAddDebugf("after: header removal is set up")

	basketAddDebugf("before: attach basket API response capture")
	resChan := attachBasketResponseCapture(page)
	basketAddDebugf("after: basket API response capture attached")

	basketAddDebugf("before: navigate to product detail page")
	if _, err := page.Goto(targetURL); err != nil {
		return fmt.Errorf("could not go to basket add URL: %w", err)
	}
	basketAddDebugf("after: product detail page navigation completed")

	basketAddDebugf("before: wait for product detail page load state")
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("basket add page did not fully load: %w", err)
	}
	basketAddDebugf("after: product detail page fully loaded")

	restoreSelector := `[data-test-id="restore-order-modal.confirm"]`
	restoreButton := page.Locator(restoreSelector).Nth(0)
	basketAddDebugf("before: wait for restore-order confirm button (%s)", restoreSelector)
	if err := restoreButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find restore-order confirm button '%s': %w", restoreSelector, err)
	}
	basketAddDebugf("after: restore-order confirm button is visible")

	basketAddDebugf("before: click restore-order confirm button")
	clickedRestore := false
	for attempt := 1; attempt <= 3; attempt++ {
		restoreButton = page.Locator(restoreSelector).Nth(0)
		if err := restoreButton.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(3000),
		}); err != nil {
			if isRetryableRestoreModalClickError(err) || isPlaywrightTimeoutError(err) {
				basketAddDebugf("after: restore-order confirm click retry needed (attempt %d): %v", attempt, err)
				time.Sleep(150 * time.Millisecond)
				continue
			}
			return fmt.Errorf("could not click restore-order confirm button '%s': %w", restoreSelector, err)
		}
		clickedRestore = true
		break
	}
	if !clickedRestore {
		return fmt.Errorf("could not click restore-order confirm button '%s' after retries", restoreSelector)
	}
	basketAddDebugf("after: restore-order confirm button clicked")

	submitSelector := `[data-test-id="product-modal.submit"]`
	submitButton := page.Locator(submitSelector).Nth(0)
	basketAddDebugf("before: wait for submit button (%s)", submitSelector)
	if err := submitButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find submit button '%s': %w", submitSelector, err)
	}
	basketAddDebugf("after: submit button is visible")

	basketAddDebugf("before: click submit button")
	clickStartedAt := time.Now()
	clickedSubmit := false
	for attempt := 1; attempt <= 3; attempt++ {
		submitButton = page.Locator(submitSelector).Nth(0)
		if err := submitButton.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(3000),
		}); err != nil {
			if isRetryableRestoreModalClickError(err) || isPlaywrightTimeoutError(err) {
				basketAddDebugf("after: submit click retry needed (attempt %d): %v", attempt, err)
				time.Sleep(150 * time.Millisecond)
				continue
			}
			return fmt.Errorf("could not click submit button '%s': %w", submitSelector, err)
		}
		clickedSubmit = true
		break
	}
	if !clickedSubmit {
		return fmt.Errorf("could not click submit button '%s' after retries", submitSelector)
	}
	basketAddDebugf("after: submit button clicked")

	basketAddDebugf("before: wait for baskets API response after submit click")
	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
	if err != nil {
		return err
	}
	basketAddDebugf("after: baskets API response received after submit click (status=%d)", basketRes.Status)

	basketAddDebugf("before: print basket add result JSON")
	printBasketActionResult("add", venueSlug, itemID, basketRes)
	basketAddDebugf("after: basket add result JSON printed")
	return nil
}

func runBasketRemove(cfg Config, venueSlug, itemID string) error {
	basketRemoveDebugf("before: start remove flow (venue_slug=%s item_id=%s)", venueSlug, itemID)
	basketRemoveDebugf("before: pre-check basket contents for venue/item")

	quantity, err := getBasketItemQuantityForVenue(cfg, venueSlug, itemID, basketRemoveDebugf)
	if err != nil {
		basketRemoveDebugf("after: basket pre-check failed: %v", err)
		return err
	}
	basketRemoveDebugf("after: basket pre-check completed (quantity=%d)", quantity)
	if quantity <= 0 {
		return fmt.Errorf("item '%s' is not present in basket for venue '%s'", itemID, venueSlug)
	}

	basketRemoveDebugf("before: route to checkout remove flow")
	if err := runBasketRemoveFromCheckout(cfg, venueSlug, itemID, quantity); err != nil {
		basketRemoveDebugf("after: checkout remove flow failed: %v", err)
		return err
	}
	basketRemoveDebugf("after: checkout remove flow completed")
	return nil
}

func isBasketItemPresentForVenue(cfg Config, venueSlug, itemID string) (bool, error) {
	quantity, err := getBasketItemQuantityForVenue(cfg, venueSlug, itemID, basketAddDebugf)
	if err != nil {
		return false, err
	}
	return quantity > 0, nil
}

func getBasketItemQuantityForVenue(cfg Config, venueSlug, itemID string, debugf func(string, ...interface{})) (int, error) {
	debug := func(format string, args ...interface{}) {
		if debugf != nil {
			debugf(format, args...)
		}
	}

	if _, err := os.Stat(cfg.UserDataDir); os.IsNotExist(err) {
		return 0, fmt.Errorf("no saved session found in '%s'. Please run the 'auth' command first", cfg.UserDataDir)
	}

	debug("before: start playwright for basket pre-check")
	pw, err := playwright.Run()
	if err != nil {
		return 0, fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()
	debug("after: playwright started for basket pre-check")

	debug("before: launch persistent browser context for basket pre-check")
	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(cfg.Headless),
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return 0, fmt.Errorf("could not launch persistent context: %w", err)
	}
	defer ctx.Close()
	debug("after: persistent browser context launched for basket pre-check")

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
	debug("before: add init script for basket pre-check")
	if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return 0, fmt.Errorf("could not add init script: %w", err)
	}
	debug("after: init script added for basket pre-check")

	debug("before: create new page for basket pre-check")
	page, err := ctx.NewPage()
	if err != nil {
		return 0, fmt.Errorf("could not create new page: %w", err)
	}
	debug("after: new page created for basket pre-check")

	debug("before: set viewport for basket pre-check")
	if err := page.SetViewportSize(1440, 810); err != nil {
		return 0, fmt.Errorf("could not set viewport size: %w", err)
	}
	debug("after: viewport set for basket pre-check")

	debug("before: set up header removal for basket pre-check")
	if err := setupHeaderRemoval(page); err != nil {
		return 0, fmt.Errorf("could not set up header removal: %w", err)
	}
	debug("after: header removal set for basket pre-check")

	debug("before: attach basket API response capture for basket pre-check")
	resChan := attachBasketResponseCapture(page)
	debug("after: basket API response capture attached for basket pre-check")

	targetURL := defaultBasketCaptureURL
	if cfg.SuccessURLPattern != "" {
		targetURL = cfg.SuccessURLPattern
	}
	debug("before: navigate for basket pre-check (url=%s)", targetURL)
	requestStartedAt := time.Now()
	if _, err := page.Goto(targetURL); err != nil {
		return 0, fmt.Errorf("could not go to basket pre-check page: %w", err)
	}
	debug("after: navigation completed for basket pre-check")

	debug("before: wait for load state in basket pre-check")
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return 0, fmt.Errorf("basket pre-check page did not fully load: %w", err)
	}
	debug("after: load state reached in basket pre-check")

	debug("before: wait for baskets API response in basket pre-check")
	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, requestStartedAt)
	if err != nil {
		return 0, err
	}
	debug("after: baskets API response received in basket pre-check (status=%d)", basketRes.Status)

	baskets := extractBasketOutputs(basketRes.JSON)
	quantity := basketItemQuantityForVenue(baskets, venueSlug, itemID)
	debug("after: basket contents inspected (venue_slug=%s item_id=%s quantity=%d)", venueSlug, itemID, quantity)
	return quantity, nil
}

func runBasketAddFromCheckout(cfg Config, venueSlug, itemID string) error {
	targetURL := buildCheckoutURL(venueSlug)
	basketAddDebugf("before: prepare checkout increment URL")
	fmt.Printf("Opening checkout page for basket add: %s\n", targetURL)
	basketAddDebugf("after: checkout increment URL ready")

	basketAddDebugf("before: start playwright for checkout increment flow")
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()
	basketAddDebugf("after: playwright started for checkout increment flow")

	basketAddDebugf("before: ensure user data dir exists (%s)", cfg.UserDataDir)
	if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
		return fmt.Errorf("could not create user data directory: %w", err)
	}
	basketAddDebugf("after: user data dir ready")

	basketAddDebugf("before: launch persistent browser context")
	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(false), // Always interactive for basket actions
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return fmt.Errorf("could not launch persistent context: %w", err)
	}
	defer ctx.Close()
	basketAddDebugf("after: persistent browser context launched")

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
	basketAddDebugf("before: add anti-detection init script")
	if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("could not add init script: %w", err)
	}
	basketAddDebugf("after: anti-detection init script added")

	basketAddDebugf("before: create new page")
	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("could not create new page: %w", err)
	}
	basketAddDebugf("after: new page created")

	basketAddDebugf("before: set viewport")
	if err := page.SetViewportSize(1440, 810); err != nil {
		return fmt.Errorf("could not set viewport size: %w", err)
	}
	basketAddDebugf("after: viewport set to 1440x810")

	basketAddDebugf("before: set up header removal")
	if err := setupHeaderRemoval(page); err != nil {
		return fmt.Errorf("could not set up header removal: %w", err)
	}
	basketAddDebugf("after: header removal is set up")

	basketAddDebugf("before: attach basket API response capture")
	resChan := attachBasketResponseCapture(page)
	basketAddDebugf("after: basket API response capture attached")

	basketAddDebugf("before: navigate to checkout page")
	if _, err := page.Goto(targetURL); err != nil {
		return fmt.Errorf("could not go to checkout URL: %w", err)
	}
	basketAddDebugf("after: checkout page navigation completed")

	basketAddDebugf("before: wait for checkout page load state")
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("checkout page did not fully load: %w", err)
	}
	basketAddDebugf("after: checkout page fully loaded")

	cartItemSelector := buildCheckoutCartItemSelector(itemID)
	basketAddDebugf("before: check if cart item exists using selector %s", cartItemSelector)
	hasCartItem, err := waitForCheckoutCartItem(page, cartItemSelector, basketCheckoutCartItemWaitTimeout(cfg.Timeout))
	if err != nil {
		return err
	}
	basketAddDebugf("after: cart item existence check completed (exists=%t)", hasCartItem)
	if !hasCartItem {
		return fmt.Errorf("item '%s' was found in basket pre-check but not found on checkout page", itemID)
	}
	fmt.Printf("Item %s is in basket. Using checkout flow for increment.\n", itemID)

	cartItemButton := page.Locator(cartItemSelector).Nth(0).Locator("button").Nth(0)
	basketAddDebugf("before: wait for cart item button")
	if err := cartItemButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find cart item button for '%s': %w", itemID, err)
	}
	basketAddDebugf("after: cart item button is visible")

	basketAddDebugf("before: click cart item button")
	if err := cartItemButton.Click(); err != nil {
		return fmt.Errorf("could not click cart item button for '%s': %w", itemID, err)
	}
	basketAddDebugf("after: cart item button clicked")

	addButtonSelector := `[data-test-id="product-modal.total-price"]`
	addButton := page.Locator(addButtonSelector).Nth(0)
	basketAddDebugf("before: wait for add button in modal")
	if err := addButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find basket add button '%s' after cart item selection: %w", addButtonSelector, err)
	}
	basketAddDebugf("after: add button in modal is visible")

	incrementButtonSelector := `[data-test-id="product-modal.quantity.increment"]`
	incrementButton := page.Locator(incrementButtonSelector).Nth(0)
	basketAddDebugf("before: wait for quantity increment button in modal")
	if err := incrementButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find quantity increment button '%s' in modal: %w", incrementButtonSelector, err)
	}
	basketAddDebugf("after: quantity increment button in modal is visible")

	basketAddDebugf("before: click quantity increment button in modal")
	if err := incrementButton.Click(); err != nil {
		return fmt.Errorf("could not click quantity increment button '%s' in modal: %w", incrementButtonSelector, err)
	}
	basketAddDebugf("after: quantity increment button in modal clicked")

	basketAddDebugf("before: click add button in modal")
	clickStartedAt := time.Now()
	if err := addButton.Click(); err != nil {
		return fmt.Errorf("could not click basket add button '%s': %w", addButtonSelector, err)
	}
	basketAddDebugf("after: add button in modal clicked")

	basketAddDebugf("before: wait for baskets API response after add click")
	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
	if err != nil {
		return err
	}
	basketAddDebugf("after: baskets API response received (status=%d)", basketRes.Status)

	basketAddDebugf("before: print basket add result JSON")
	printBasketActionResult("add", venueSlug, itemID, basketRes)
	basketAddDebugf("after: basket add result JSON printed")
	return nil
}

func runBasketRemoveFromCheckout(cfg Config, venueSlug, itemID string, quantity int) error {
	targetURL := buildCheckoutURL(venueSlug)
	basketRemoveDebugf("before: prepare checkout remove URL")
	fmt.Printf("Opening checkout page for basket remove: %s\n", targetURL)
	basketRemoveDebugf("after: checkout remove URL ready")

	basketRemoveDebugf("before: start playwright for checkout remove flow")
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()
	basketRemoveDebugf("after: playwright started for checkout remove flow")

	basketRemoveDebugf("before: ensure user data dir exists (%s)", cfg.UserDataDir)
	if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
		return fmt.Errorf("could not create user data directory: %w", err)
	}
	basketRemoveDebugf("after: user data dir ready")

	basketRemoveDebugf("before: launch persistent browser context")
	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(false), // Always interactive for basket actions
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return fmt.Errorf("could not launch persistent context: %w", err)
	}
	defer ctx.Close()
	basketRemoveDebugf("after: persistent browser context launched")

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
	basketRemoveDebugf("before: add anti-detection init script")
	if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("could not add init script: %w", err)
	}
	basketRemoveDebugf("after: anti-detection init script added")

	basketRemoveDebugf("before: create new page")
	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("could not create new page: %w", err)
	}
	basketRemoveDebugf("after: new page created")

	basketRemoveDebugf("before: set viewport")
	if err := page.SetViewportSize(1440, 810); err != nil {
		return fmt.Errorf("could not set viewport size: %w", err)
	}
	basketRemoveDebugf("after: viewport set to 1440x810")

	basketRemoveDebugf("before: set up header removal")
	if err := setupHeaderRemoval(page); err != nil {
		return fmt.Errorf("could not set up header removal: %w", err)
	}
	basketRemoveDebugf("after: header removal is set up")

	basketRemoveDebugf("before: attach basket API response capture")
	resChan := attachBasketResponseCapture(page)
	basketRemoveDebugf("after: basket API response capture attached")

	basketRemoveDebugf("before: navigate to checkout page")
	if _, err := page.Goto(targetURL); err != nil {
		return fmt.Errorf("could not go to checkout URL: %w", err)
	}
	basketRemoveDebugf("after: checkout page navigation completed")

	basketRemoveDebugf("before: wait for checkout page load state")
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("checkout page did not fully load: %w", err)
	}
	basketRemoveDebugf("after: checkout page fully loaded")

	cartItemSelector := buildCheckoutCartItemSelector(itemID)
	basketRemoveDebugf("before: check if cart item exists using selector %s", cartItemSelector)
	hasCartItem, err := waitForCheckoutCartItem(page, cartItemSelector, basketCheckoutCartItemWaitTimeout(cfg.Timeout))
	if err != nil {
		return err
	}
	basketRemoveDebugf("after: cart item existence check completed (exists=%t)", hasCartItem)
	if !hasCartItem {
		return fmt.Errorf("item '%s' was found in basket pre-check but not found on checkout page", itemID)
	}
	fmt.Printf("Item %s is in basket with quantity %d. Using checkout flow for removal.\n", itemID, quantity)

	cartItemButton := page.Locator(cartItemSelector).Nth(0).Locator("button").Nth(0)
	basketRemoveDebugf("before: wait for cart item button")
	if err := cartItemButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find cart item button for '%s': %w", itemID, err)
	}
	basketRemoveDebugf("after: cart item button is visible")

	basketRemoveDebugf("before: click cart item button")
	if err := cartItemButton.Click(); err != nil {
		return fmt.Errorf("could not click cart item button for '%s': %w", itemID, err)
	}
	basketRemoveDebugf("after: cart item button clicked")

	decrementButtonSelector := `[data-test-id="product-modal.quantity.decrement"]`
	decrementButton := page.Locator(decrementButtonSelector).Nth(0)
	basketRemoveDebugf("before: wait for quantity decrement button in modal")
	if err := decrementButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find quantity decrement button '%s' in modal: %w", decrementButtonSelector, err)
	}
	basketRemoveDebugf("after: quantity decrement button in modal is visible")

	for i := 0; i < quantity; i++ {
		basketRemoveDebugf("before: click quantity decrement button in modal (%d/%d)", i+1, quantity)
		clicked := false
		for attempt := 1; attempt <= 3; attempt++ {
			decrementButton = page.Locator(decrementButtonSelector).Nth(0)
			if err := decrementButton.Click(playwright.LocatorClickOptions{
				Timeout: playwright.Float(3000),
			}); err != nil {
				if isRetryableRestoreModalClickError(err) || isPlaywrightTimeoutError(err) {
					basketRemoveDebugf("after: decrement click retry needed (%d/%d, attempt %d): %v", i+1, quantity, attempt, err)
					time.Sleep(150 * time.Millisecond)
					continue
				}
				return fmt.Errorf("could not click quantity decrement button '%s' in modal on click %d/%d: %w", decrementButtonSelector, i+1, quantity, err)
			}
			clicked = true
			break
		}
		if !clicked {
			return fmt.Errorf("could not click quantity decrement button '%s' in modal on click %d/%d after retries", decrementButtonSelector, i+1, quantity)
		}
		basketRemoveDebugf("after: quantity decrement button in modal clicked (%d/%d)", i+1, quantity)
	}

	submitButtonSelector := `[data-test-id="product-modal.submit"]`
	submitButton := page.Locator(submitButtonSelector).Nth(0)
	basketRemoveDebugf("before: wait for modal submit button")
	if err := submitButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find modal submit button '%s': %w", submitButtonSelector, err)
	}
	basketRemoveDebugf("after: modal submit button is visible")

	basketRemoveDebugf("before: click modal submit button")
	clickStartedAt := time.Now()
	if err := submitButton.Click(); err != nil {
		return fmt.Errorf("could not click modal submit button '%s': %w", submitButtonSelector, err)
	}
	basketRemoveDebugf("after: modal submit button clicked")

	basketRemoveDebugf("before: wait for baskets API response after remove submit")
	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
	if err != nil {
		return err
	}
	basketRemoveDebugf("after: baskets API response received (status=%d)", basketRes.Status)

	basketRemoveDebugf("before: print basket remove result JSON")
	printBasketActionResult("remove", venueSlug, itemID, basketRes)
	basketRemoveDebugf("after: basket remove result JSON printed")
	return nil
}

func runBasketItemAction(cfg Config, venueSlug, itemID, buttonSelector, actionName string) error {
	targetURL := buildBasketAddURL(venueSlug, itemID)
	fmt.Printf("Opening basket %s page: %s\n", actionName, targetURL)
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: start direct item-page add flow URL setup complete")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: start playwright for direct item-page add")
	}
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: playwright started for direct item-page add")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: ensure user data dir exists (%s)", cfg.UserDataDir)
	}
	if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
		return fmt.Errorf("could not create user data directory: %w", err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: user data dir ready")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: launch persistent browser context")
	}
	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(false), // Always interactive for basket actions
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return fmt.Errorf("could not launch persistent context: %w", err)
	}
	defer ctx.Close()
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: persistent browser context launched")
	}

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
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: add anti-detection init script")
	}
	if err := ctx.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("could not add init script: %w", err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: anti-detection init script added")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: create new page")
	}
	page, err := ctx.NewPage()
	if err != nil {
		return fmt.Errorf("could not create new page: %w", err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: new page created")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: set viewport")
	}
	if err := page.SetViewportSize(1440, 810); err != nil {
		return fmt.Errorf("could not set viewport size: %w", err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: viewport set to 1440x810")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: set up header removal")
	}
	if err := setupHeaderRemoval(page); err != nil {
		return fmt.Errorf("could not set up header removal: %w", err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: header removal is set up")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: attach basket API response capture")
	}
	resChan := attachBasketResponseCapture(page)
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: basket API response capture attached")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: navigate to item page")
	}
	if _, err := page.Goto(targetURL); err != nil {
		return fmt.Errorf("could not go to basket %s URL: %w", actionName, err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: item page navigation completed")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: wait for item page load state")
	}
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("basket %s page did not fully load: %w", actionName, err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: item page fully loaded")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: check restore order modal")
	}
	restoreClicked, err := maybeConfirmRestoreOrderModal(page, cfg.Timeout)
	if err != nil {
		return err
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: restore order modal check completed (clicked=%t)", restoreClicked)
	}
	if restoreClicked {
		if strings.EqualFold(actionName, "add") {
			submitAddSelector := `[data-test-id="product-modal.submit"]`
			submitAddButton := page.Locator(submitAddSelector).Nth(0)

			basketAddDebugf("before: click submit add button after restore confirmation")
			clickStartedAt := time.Now()
			if err := submitAddButton.Click(); err != nil {
				return fmt.Errorf("could not click submit add button '%s' after restore confirmation: %w", submitAddSelector, err)
			}
			basketAddDebugf("after: submit add button clicked after restore confirmation")

			basketAddDebugf("before: wait for baskets API response after submit add click")
			basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
			if err != nil {
				return err
			}
			basketAddDebugf("after: baskets API response received after submit add click (status=%d)", basketRes.Status)

			basketAddDebugf("before: print basket add result JSON after submit add flow")
			printBasketActionResult(actionName, venueSlug, itemID, basketRes)
			basketAddDebugf("after: basket add result JSON printed after submit add flow")
			return nil
		}
	}

	button := page.Locator(buttonSelector).Nth(0)
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: wait for add button (%s)", buttonSelector)
	}
	if err := button.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find basket %s button '%s': %w", actionName, buttonSelector, err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: add button is visible")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: click add button")
	}
	clickStartedAt := time.Now()
	if err := button.Click(); err != nil {
		return fmt.Errorf("could not click basket %s button '%s': %w", actionName, buttonSelector, err)
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: add button clicked")
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: wait for baskets API response")
	}
	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
	if err != nil {
		return err
	}
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: baskets API response received (status=%d)", basketRes.Status)
	}

	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("before: print basket add result JSON")
	}
	printBasketActionResult(actionName, venueSlug, itemID, basketRes)
	if strings.EqualFold(actionName, "add") {
		basketAddDebugf("after: basket add result JSON printed")
	}
	return nil
}

func basketAddDebugf(format string, args ...interface{}) {
	fmt.Printf("[DEBUG][basket add] "+format+"\n", args...)
}

func basketRemoveDebugf(format string, args ...interface{}) {
	fmt.Printf("[DEBUG][basket remove] "+format+"\n", args...)
}

func printBasketActionResult(actionName, venueSlug, itemID string, basketRes basketResponseData) {
	baskets := extractBasketOutputs(basketRes.JSON)

	result := map[string]interface{}{
		"action":     actionName,
		"venue_slug": venueSlug,
		"item_id":    itemID,
		"request": map[string]interface{}{
			"url":    basketRes.URL,
			"status": basketRes.Status,
		},
		"count":   len(baskets),
		"baskets": baskets,
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))
}

func runCheckout(cfg Config, venueSlug string) error {
	targetURL := buildCheckoutURL(venueSlug)
	fmt.Printf("Opening checkout page: %s\n", targetURL)

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	defer pw.Stop()

	if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
		return fmt.Errorf("could not create user data directory: %w", err)
	}

	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(false), // Always interactive for checkout
		UserAgent: playwright.String("Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"),
	})
	if err != nil {
		return fmt.Errorf("could not launch persistent context: %w", err)
	}
	defer ctx.Close()

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

	if _, err := page.Goto(targetURL); err != nil {
		return fmt.Errorf("could not go to checkout URL: %w", err)
	}

	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("checkout page did not fully load: %w", err)
	}

	sendOrderButtonSelector := `[data-test-id="SendOrderButton"]`
	sendOrderButton := page.Locator(sendOrderButtonSelector).Nth(0)
	if err := sendOrderButton.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("could not find Send Order button '%s': %w", sendOrderButtonSelector, err)
	}

	if err := sendOrderButton.Click(); err != nil {
		return fmt.Errorf("could not click Send Order button '%s': %w", sendOrderButtonSelector, err)
	}

	modalInnerValue, foundModal, err := waitForGenericCheckoutErrorModalInnerText(page, 10*time.Second)
	if err != nil {
		return err
	}

	result := map[string]interface{}{
		"venue_slug":                   venueSlug,
		"send_order_clicked":           true,
		"generic_checkout_error_modal": nil,
	}
	if foundModal {
		result["generic_checkout_error_modal"] = modalInnerValue
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))
	return nil
}

func attachBasketResponseCapture(page playwright.Page) <-chan basketResponseEvent {
	resChan := make(chan basketResponseEvent, 10)

	page.OnResponse(func(res playwright.Response) {
		if !isBasketPageRequest(res.Request().Method(), res.URL()) {
			return
		}

		go func() {
			b, err := res.Body()
			select {
			case resChan <- basketResponseEvent{
				body:   b,
				err:    err,
				url:    res.URL(),
				status: res.Status(),
				at:     time.Now(),
			}:
			default:
				// Keep processing without blocking the event loop.
			}
		}()
	})

	return resChan
}

func waitForBasketAPIResponse(resChan <-chan basketResponseEvent, timeout time.Duration, minTime time.Time) (basketResponseData, error) {
	var (
		result   basketResponseData
		lastErr  error
		hasValid bool
	)
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	var debounceTimer *time.Timer
	var debounceC <-chan time.Time
	resetDebounce := func() {
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(1 * time.Second)
			debounceC = debounceTimer.C
			return
		}

		if !debounceTimer.Stop() {
			select {
			case <-debounceTimer.C:
			default:
			}
		}
		debounceTimer.Reset(1 * time.Second)
		debounceC = debounceTimer.C
	}
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()

	for {
		select {
		case res := <-resChan:
			if res.at.Before(minTime) {
				continue
			}
			if res.err != nil {
				lastErr = res.err
				continue
			}
			if res.status < 200 || res.status >= 300 {
				lastErr = fmt.Errorf("received non-success baskets response status %d from %s", res.status, res.url)
				continue
			}

			var responseJSON interface{}
			if err := json.Unmarshal(res.body, &responseJSON); err != nil {
				lastErr = err
				continue
			}

			result = basketResponseData{
				URL:    res.url,
				Status: res.status,
				JSON:   responseJSON,
			}
			hasValid = true
			resetDebounce()
		case <-debounceC:
			return result, nil
		case <-timeoutTimer.C:
			if hasValid {
				return result, nil
			}
			if lastErr != nil {
				return result, fmt.Errorf("timed out waiting for baskets API JSON response, last error: %w", lastErr)
			}
			return result, fmt.Errorf("timed out waiting for API response from %s", basketAPIURL)
		}
	}
}

func extractBasketOutputs(responseJSON interface{}) []BasketOutput {
	responseMap, ok := responseJSON.(map[string]interface{})
	if !ok {
		return nil
	}

	var basketsRaw []interface{}
	if basketMap := asMap(responseMap["basket"]); basketMap != nil {
		if arr, ok := basketMap["baskets"].([]interface{}); ok {
			basketsRaw = arr
		}
	}
	if basketsRaw == nil {
		if arr, ok := responseMap["baskets"].([]interface{}); ok {
			basketsRaw = arr
		}
	}
	if basketsRaw == nil {
		basketsRaw = findNestedArrayByKey(responseMap, "baskets")
	}
	if len(basketsRaw) == 0 {
		return nil
	}

	outputs := make([]BasketOutput, 0, len(basketsRaw))
	for _, basketRaw := range basketsRaw {
		basketMap := asMap(basketRaw)
		if basketMap == nil {
			continue
		}

		output := BasketOutput{
			ID:        firstStringFromScope(basketMap, []string{"id", "basket_id"}),
			Total:     extractBasketTotal(basketMap),
			VenueSlug: extractBasketVenueSlug(basketMap),
			Items:     extractBasketItemOutputs(basketMap),
		}
		outputs = append(outputs, output)
	}

	return outputs
}

func basketContainsVenueItem(baskets []BasketOutput, venueSlug, itemID string) bool {
	return basketItemQuantityForVenue(baskets, venueSlug, itemID) > 0
}

func basketItemQuantityForVenue(baskets []BasketOutput, venueSlug, itemID string) int {
	wantVenue := strings.TrimSpace(strings.ToLower(venueSlug))
	wantItem := strings.TrimSpace(strings.ToLower(itemID))
	if wantVenue == "" || wantItem == "" {
		return 0
	}

	totalCount := 0
	for _, basket := range baskets {
		if strings.TrimSpace(strings.ToLower(basket.VenueSlug)) != wantVenue {
			continue
		}
		for _, item := range basket.Items {
			if strings.TrimSpace(strings.ToLower(item.ID)) == wantItem {
				if item.Count > 0 {
					totalCount += item.Count
				} else {
					totalCount++
				}
			}
		}
	}
	return totalCount
}

func extractBasketTotal(basketMap map[string]interface{}) interface{} {
	if total, ok := basketMap["total"]; ok {
		return total
	}
	if telemetry := asMap(basketMap["telemetry"]); telemetry != nil {
		if total, ok := telemetry["basket_total"]; ok {
			return total
		}
	}
	return nil
}

func extractBasketVenueSlug(basketMap map[string]interface{}) string {
	if venueMap := asMap(basketMap["venue"]); venueMap != nil {
		if slug := firstStringFromScope(venueMap, []string{"slug", "venue_slug"}); slug != "" {
			return slug
		}
	}
	return firstStringFromScope(basketMap, []string{"venue_slug", "slug"})
}

func extractBasketItemOutputs(basketMap map[string]interface{}) []BasketItemOutput {
	var itemsRaw []interface{}
	if arr, ok := basketMap["items"].([]interface{}); ok {
		itemsRaw = arr
	} else {
		itemsRaw = findNestedArrayByKey(basketMap, "items")
	}
	if len(itemsRaw) == 0 {
		return nil
	}

	items := make([]BasketItemOutput, 0, len(itemsRaw))
	for _, itemRaw := range itemsRaw {
		itemMap := asMap(itemRaw)
		if itemMap == nil {
			continue
		}

		item := BasketItemOutput{
			ID:       firstStringFromScope(itemMap, []string{"id", "item_id", "product_id"}),
			Name:     firstDisplayStringFromScope(itemMap, []string{"name", "title"}),
			ImageURL: extractBasketItemImageURL(itemMap),
		}

		if count, ok := toInt(itemMap["count"]); ok {
			item.Count = count
		}
		if isAvailable, ok := toBool(itemMap["is_available"]); ok {
			item.IsAvailable = isAvailable
		}
		if price, ok := itemMap["price"]; ok {
			item.Price = normalizePrice(price)
		}
		item.Total = calculateBasketItemTotal(item.Count, item.Price)
		items = append(items, item)
	}

	return items
}

func extractBasketItemImageURL(itemMap map[string]interface{}) string {
	if imageMap := asMap(itemMap["image"]); imageMap != nil {
		if imageURL := firstStringFromScope(imageMap, []string{"url", "image_url"}); imageURL != "" {
			return imageURL
		}
	}
	if imageURL := firstStringFromScope(itemMap, []string{"image_url"}); imageURL != "" {
		return imageURL
	}
	if rawImage, ok := itemMap["image"]; ok {
		return toString(rawImage)
	}
	return ""
}

func calculateBasketItemTotal(count int, price interface{}) interface{} {
	if count <= 0 {
		return nil
	}

	priceValue, ok := toFloat64(price)
	if !ok {
		return nil
	}

	total := float64(count) * priceValue
	if total == float64(int64(total)) {
		return int64(total)
	}
	return total
}

func toInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		if i, err := strconv.Atoi(s); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f), true
		}
	}
	return 0, false
}

func toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

func toBool(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		switch s {
		case "true", "1", "yes", "y":
			return true, true
		case "false", "0", "no", "n":
			return false, true
		default:
			return false, false
		}
	case int, int8, int16, int32, int64:
		i, _ := toInt(v)
		return i != 0, true
	case uint, uint8, uint16, uint32, uint64:
		i, _ := toInt(v)
		return i != 0, true
	case float32, float64:
		f, _ := toFloat64(v)
		return f != 0, true
	}
	return false, false
}

func isBasketPageRequest(method, requestURL string) bool {
	if !strings.EqualFold(method, "GET") {
		return false
	}

	return strings.Contains(requestURL, basketAPIURL)
}

func maybeConfirmRestoreOrderModal(page playwright.Page, timeout time.Duration) (bool, error) {
	selector := `[data-test-id="restore-order-modal.confirm"]`
	waitTimeout := basketRestoreModalWaitTimeout(timeout)
	deadline := time.Now().Add(waitTimeout + basketRestoreModalExtraWaitTimeout())
	button := page.Locator(selector).Nth(0)

	for time.Now().Before(deadline) {
		waitChunk := minDuration(3*time.Second, time.Until(deadline))
		if waitChunk <= 0 {
			break
		}

		if err := button.WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(float64(waitChunk.Milliseconds())),
		}); err != nil {
			if isPlaywrightTimeoutError(err) {
				continue
			}
			return false, fmt.Errorf("could not inspect restore-order confirm button '%s': %w", selector, err)
		}

		clickChunk := minDuration(3*time.Second, time.Until(deadline))
		if clickChunk <= 0 {
			break
		}

		if err := button.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(float64(clickChunk.Milliseconds())),
		}); err != nil {
			if isRetryableRestoreModalClickError(err) || isPlaywrightTimeoutError(err) {
				time.Sleep(150 * time.Millisecond)
				continue
			}
			return false, fmt.Errorf("could not click restore-order confirm button '%s': %w", selector, err)
		}

		return true, nil
	}

	return false, nil
}

func basketRestoreModalWaitTimeout(timeout time.Duration) time.Duration {
	const maxWait = 30 * time.Second
	if timeout <= 0 {
		return maxWait
	}
	if timeout < maxWait {
		return timeout
	}
	return maxWait
}

func basketRestoreModalExtraWaitTimeout() time.Duration {
	return 10 * time.Second
}

func isPlaywrightTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func isRetryableRestoreModalClickError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	retryableSnippets := []string{
		"not attached to the dom",
		"element is not attached",
		"element is detached",
	}
	for _, snippet := range retryableSnippets {
		if strings.Contains(message, snippet) {
			return true
		}
	}
	return false
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func waitForCheckoutCartItem(page playwright.Page, cartItemSelector string, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	locator := page.Locator(cartItemSelector).Nth(0)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		count, err := locator.Count()
		if err != nil {
			return false, fmt.Errorf("could not inspect checkout cart item '%s': %w", cartItemSelector, err)
		}
		if count > 0 {
			return true, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return false, nil
}

func basketCheckoutCartItemWaitTimeout(timeout time.Duration) time.Duration {
	const maxWait = 10 * time.Second
	if timeout <= 0 {
		return maxWait
	}
	if timeout < maxWait {
		return timeout
	}
	return maxWait
}

func buildCheckoutCartItemSelector(itemID string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(itemID)
	return fmt.Sprintf(`div[data-test-id="CartItem"][data-value="%s"]`, escaped)
}

func buildBasketAddURL(venueSlug, itemID string) string {
	return fmt.Sprintf(
		"https://wolt.com/en/lva/riga/venue/%s/itemid-%s",
		url.PathEscape(venueSlug),
		url.PathEscape(itemID),
	)
}

func buildCheckoutURL(venueSlug string) string {
	return fmt.Sprintf(
		"https://wolt.com/en/lva/riga/venue/%s/checkout",
		url.PathEscape(venueSlug),
	)
}

func waitForGenericCheckoutErrorModalInnerText(page playwright.Page, timeout time.Duration) (string, bool, error) {
	selector := `div[data-test-id="GenericCheckoutErrorModal"], div.GenericCheckoutErrorModal, [data-test-id="GenericCheckoutErrorModal"]`
	modal := page.Locator(selector).Nth(0)

	if err := modal.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(timeout.Milliseconds())),
	}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "timeout") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("could not check GenericCheckoutErrorModal visibility: %w", err)
	}

	innerValue, err := modal.InnerText()
	if err != nil {
		return "", false, fmt.Errorf("could not read GenericCheckoutErrorModal inner value: %w", err)
	}

	return strings.TrimSpace(innerValue), true, nil
}

type SearchProduct struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Price     interface{} `json:"price"`
	VenueID   string      `json:"venue_id"`
	VenueSlug string      `json:"venue_slug"`
}

func extractSearchProducts(responseJSON interface{}) []SearchProduct {
	resMap, ok := responseJSON.(map[string]interface{})
	if !ok {
		return nil
	}

	sections, ok := resMap["sections"].([]interface{})
	if !ok {
		if nestedSections := findNestedArrayByKey(resMap, "sections"); nestedSections != nil {
			sections = nestedSections
		} else {
			return nil
		}
	}

	products := make([]SearchProduct, 0)
	seen := make(map[string]struct{})

	for sectionIdx, sectionRaw := range sections {
		section, ok := sectionRaw.(map[string]interface{})
		if !ok {
			continue
		}
		items, ok := section["items"].([]interface{})
		if !ok {
			continue
		}

		for itemIdx, itemRaw := range items {
			item, ok := itemRaw.(map[string]interface{})
			if !ok {
				continue
			}

			product, ok := extractSearchProduct(item, sectionIdx, itemIdx)
			if !ok {
				continue
			}

			key := product.ID + "|" + product.VenueID + "|" + product.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			products = append(products, product)
		}
	}

	return products
}

func extractSearchProduct(item map[string]interface{}, sectionIdx, itemIdx int) (SearchProduct, bool) {
	scopes := buildProductScopes(item)

	id := firstStringFromScopes(scopes, []string{"id", "item_id", "product_id", "public_id", "reference_id"})
	name := firstDisplayStringFromScopes(scopes, []string{"name", "title", "item_name", "display_name", "product_name"})
	price, hasPrice := firstPriceFromScopes(scopes)

	venueID := ""
	venueSlug := ""

	for _, scope := range scopes {
		if venueID == "" {
			venueID = firstStringFromScope(scope, []string{"venue_id"})
		}
		if venueSlug == "" {
			venueSlug = firstStringFromScope(scope, []string{"venue_slug"})
		}
		if venueID != "" && venueSlug != "" {
			break
		}
	}

	if venue := findNestedMapByKey(item, "venue"); venue != nil {
		if venueID == "" {
			venueID = firstStringFromScope(venue, []string{"id", "venue_id"})
		}
		if venueSlug == "" {
			venueSlug = firstStringFromScope(venue, []string{"slug", "venue_slug"})
		}

		if nestedVenue := asMap(venue["value"]); nestedVenue != nil {
			if venueID == "" {
				venueID = firstStringFromScope(nestedVenue, []string{"id", "venue_id"})
			}
			if venueSlug == "" {
				venueSlug = firstStringFromScope(nestedVenue, []string{"slug", "venue_slug"})
			}
		}
	}

	textValues := collectStringValues(item)
	if name == "" {
		name = fallbackNameFromTextValues(textValues)
	}
	for _, text := range textValues {
		if id == "" {
			id = extractIDFromText(text)
		}
		if venueSlug == "" {
			venueSlug = extractVenueSlugFromText(text)
		}
		if id != "" && venueSlug != "" {
			break
		}
	}

	if name == "" {
		return SearchProduct{}, false
	}
	if id == "" {
		id = fmt.Sprintf("section_%d_item_%d", sectionIdx, itemIdx)
	}

	var normalizedPrice interface{}
	if hasPrice {
		normalizedPrice = normalizePrice(price)
	}

	return SearchProduct{
		ID:        id,
		Name:      name,
		Price:     normalizedPrice,
		VenueID:   venueID,
		VenueSlug: venueSlug,
	}, true
}

func buildProductScopes(item map[string]interface{}) []map[string]interface{} {
	scopes := make([]map[string]interface{}, 0, 8)

	for _, key := range []string{"value", "item", "product", "content", "data", "details", "menu_item", "link", "menu_item_details"} {
		if scope := asMap(item[key]); scope != nil {
			scopes = append(scopes, scope)
		}
	}

	if value := asMap(item["value"]); value != nil {
		for _, key := range []string{"item", "product", "content", "menu_item", "link", "menu_item_details"} {
			if scope := asMap(value[key]); scope != nil {
				scopes = append(scopes, scope)
			}
		}
	}

	if link := asMap(item["link"]); link != nil {
		for _, key := range []string{"menu_item_details", "action_link"} {
			if scope := asMap(link[key]); scope != nil {
				scopes = append(scopes, scope)
			}
		}
	}

	scopes = append(scopes, item)
	return scopes
}

func firstPriceFromScopes(scopes []map[string]interface{}) (interface{}, bool) {
	for _, scope := range scopes {
		if raw, ok := firstAnyFromScope(scope, []string{
			"price",
			"baseprice",
			"base_price",
			"current_price",
			"display_price",
			"formatted_price",
		}); ok {
			return raw, true
		}

		if pricing, ok := scope["pricing"]; ok {
			if value, ok := extractPriceFromValue(pricing); ok {
				return value, true
			}
		}
	}
	return nil, false
}

func firstStringFromScopes(scopes []map[string]interface{}, keys []string) string {
	for _, scope := range scopes {
		if value := firstStringFromScope(scope, keys); value != "" {
			return value
		}
	}
	return ""
}

func firstDisplayStringFromScopes(scopes []map[string]interface{}, keys []string) string {
	for _, scope := range scopes {
		if value := firstDisplayStringFromScope(scope, keys); value != "" {
			return value
		}
	}
	return ""
}

func firstStringFromScope(scope map[string]interface{}, keys []string) string {
	for _, key := range keys {
		raw, exists := scope[key]
		if !exists {
			continue
		}
		if text := toString(raw); text != "" {
			return text
		}
	}
	return ""
}

func firstDisplayStringFromScope(scope map[string]interface{}, keys []string) string {
	for _, key := range keys {
		raw, exists := scope[key]
		if !exists {
			continue
		}
		if text := toDisplayString(raw); text != "" {
			return text
		}
	}
	return ""
}

func firstAnyFromScope(scope map[string]interface{}, keys []string) (interface{}, bool) {
	for _, key := range keys {
		raw, exists := scope[key]
		if exists {
			return raw, true
		}
	}
	return nil, false
}

func extractPriceFromValue(value interface{}) (interface{}, bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"amount", "value", "current", "price", "formatted", "display"} {
			if raw, ok := v[key]; ok {
				return raw, true
			}
		}
	case []interface{}:
		for _, item := range v {
			if nested, ok := extractPriceFromValue(item); ok {
				return nested, true
			}
		}
	}
	return nil, false
}

func normalizePrice(value interface{}) interface{} {
	if nested, ok := extractPriceFromValue(value); ok {
		return nested
	}
	return value
}

func asMap(value interface{}) map[string]interface{} {
	if mapped, ok := value.(map[string]interface{}); ok {
		return mapped
	}
	return nil
}

func findNestedMapByKey(value interface{}, key string) map[string]interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		if candidate, ok := v[key]; ok {
			if mapped, ok := candidate.(map[string]interface{}); ok {
				return mapped
			}
		}
		for _, nested := range v {
			if result := findNestedMapByKey(nested, key); result != nil {
				return result
			}
		}
	case []interface{}:
		for _, nested := range v {
			if result := findNestedMapByKey(nested, key); result != nil {
				return result
			}
		}
	}
	return nil
}

func findNestedArrayByKey(value interface{}, key string) []interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		if candidate, ok := v[key]; ok {
			if arr, ok := candidate.([]interface{}); ok {
				return arr
			}
		}
		for _, nested := range v {
			if result := findNestedArrayByKey(nested, key); result != nil {
				return result
			}
		}
	case []interface{}:
		for _, nested := range v {
			if result := findNestedArrayByKey(nested, key); result != nil {
				return result
			}
		}
	}
	return nil
}

func collectStringValues(value interface{}) []string {
	values := make([]string, 0, 16)
	var walk func(interface{})

	walk = func(current interface{}) {
		switch v := current.(type) {
		case string:
			text := strings.TrimSpace(v)
			if text != "" {
				values = append(values, text)
			}
		case map[string]interface{}:
			for _, nested := range v {
				walk(nested)
			}
		case []interface{}:
			for _, nested := range v {
				walk(nested)
			}
		}
	}

	walk(value)
	return values
}

func fallbackNameFromTextValues(values []string) string {
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if strings.Contains(v, "://") || strings.HasPrefix(v, "/") {
			continue
		}
		if strings.Contains(strings.ToLower(v), "itemid-") {
			continue
		}
		if strings.HasPrefix(v, "€") || strings.HasPrefix(v, "$") || strings.HasPrefix(v, "£") {
			continue
		}
		if len(v) < 3 {
			continue
		}
		return v
	}
	return ""
}

func extractIDFromText(text string) string {
	candidates := []string{text}
	if decoded := decodeText(text); decoded != text {
		candidates = append(candidates, decoded)
	}

	for _, candidate := range candidates {
		if id := extractTokenAfterMarker(candidate, "itemid-"); id != "" {
			return id
		}
		if id := extractSegmentAfterMarker(candidate, "/item/"); id != "" {
			return id
		}
	}
	return ""
}

func extractVenueSlugFromText(text string) string {
	candidates := []string{text}
	if decoded := decodeText(text); decoded != text {
		candidates = append(candidates, decoded)
	}

	for _, candidate := range candidates {
		if slug := extractSegmentAfterMarker(candidate, "/venue/"); slug != "" {
			return slug
		}
	}
	return ""
}

func decodeText(text string) string {
	decoded, err := url.QueryUnescape(text)
	if err == nil {
		return decoded
	}
	return text
}

func extractTokenAfterMarker(text, marker string) string {
	idx := strings.Index(strings.ToLower(text), strings.ToLower(marker))
	if idx < 0 {
		return ""
	}

	start := idx + len(marker)
	end := start

	for end < len(text) {
		c := text[end]
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' {
			end++
			continue
		}
		break
	}

	if end <= start {
		return ""
	}

	return text[start:end]
}

func extractSegmentAfterMarker(text, marker string) string {
	idx := strings.Index(strings.ToLower(text), strings.ToLower(marker))
	if idx < 0 {
		return ""
	}

	start := idx + len(marker)
	if start >= len(text) {
		return ""
	}

	end := start
	for end < len(text) {
		c := text[end]
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' {
			end++
			continue
		}
		break
	}

	if end <= start {
		return ""
	}

	return text[start:end]
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	default:
		return ""
	}
}

func toDisplayString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		for _, key := range []string{"text", "value", "name", "title", "label"} {
			if raw, ok := v[key]; ok {
				if text := toDisplayString(raw); text != "" {
					return text
				}
			}
		}
		return ""
	case []interface{}:
		for _, item := range v {
			if text := toDisplayString(item); text != "" {
				return text
			}
		}
		return ""
	default:
		return toString(value)
	}
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
