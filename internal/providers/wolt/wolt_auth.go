package wolt

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

func validateAuthURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("auth URL is incorrect: domain must be wolt.com")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("auth URL is incorrect: domain must be wolt.com")
	}

	hostname := strings.ToLower(parsed.Hostname())
	if hostname != "wolt.com" && !strings.HasSuffix(hostname, ".wolt.com") {
		return "", fmt.Errorf("auth URL is incorrect: domain must be wolt.com")
	}

	return trimmed, nil
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

func runAuth(cfg Config, eraseData bool, authURL string) error {
	if eraseData {
		if err := validateEraseUserDataDir(cfg.UserDataDir); err != nil {
			return err
		}
		if err := os.RemoveAll(cfg.UserDataDir); err != nil {
			return fmt.Errorf("failed to erase user data directory: %w", err)
		}
	}

	pw, ctx, page, err := launchPersistentSession(cfg, browserSessionOptions{
		headless:         cfg.Headless,
		ensureProfileDir: true,
	})
	if err != nil {
		return err
	}
	defer closeSessionWithWarnings(pw, ctx)
	if _, err = page.Goto(authURL); err != nil {
		return fmt.Errorf("could not go to auth URL: %w", err)
	}

	// Wait for the page to be fully loaded (including network idle)
	if err := waitForPageLoadState(page, playwright.LoadStateNetworkidle, cfg.Timeout); err != nil {
		return fmt.Errorf("failed to wait for page network idle: %w", err)
	}

	// Simulate human-like mouse movements
	_ = simulateHumanBehavior(page)

	// Wait for the page to be fully loaded (including network idle) after potential redirects/JS execution
	if err := waitForPageLoadState(page, playwright.LoadStateNetworkidle, cfg.Timeout); err != nil {
		return fmt.Errorf("failed to wait for page network idle after human simulation: %w", err)
	}

	// Attempt to click the "Use only necessary" button first, if it exists
	useNecessaryButtonLocator := page.Locator("text=Use only necessary").Nth(0)
	if err := waitForVisibleLocator(useNecessaryButtonLocator, 10*time.Second); err == nil {
		if err := useNecessaryButtonLocator.Click(); err == nil {
			if err := waitForPageLoadState(page, playwright.LoadStateNetworkidle, cfg.Timeout); err != nil {
				// Non-critical wait; continue flow.
			}
		}
	}

	// Click the decline button first, if it exists
	declineButtonSelector := "[data-test-id=\"decline-button\"]"
	declineButtonLocator := page.Locator(declineButtonSelector).Nth(0)
	if err := waitForVisibleLocator(declineButtonLocator, 10*time.Second); err == nil {
		if err := declineButtonLocator.Click(); err == nil {
			// Wait for page to settle after click
			if err := waitForPageLoadState(page, playwright.LoadStateNetworkidle, cfg.Timeout); err != nil {
				// Non-critical wait; continue flow.
			}
		}
	}

	// Find the confirm button and click it
	confirmButtonSelector := "[data-test-id=\"magic-login-landing.confirm\"]"
	confirmButton, err := waitForFirstVisibleLocatorWithMessage(
		page,
		confirmButtonSelector,
		cfg.Timeout,
		fmt.Sprintf("could not find or wait for confirm button '%s'", confirmButtonSelector),
	)
	if err != nil {
		return err
	}
	if err := clickLocatorWithMessage(confirmButton, fmt.Sprintf("could not click confirm button '%s'", confirmButtonSelector)); err != nil {
		return err
	}

	// Wait for the page to reload into the success URL
	successURL := "https://wolt.com/en/discovery"
	if err := page.WaitForURL(successURL, playwright.PageWaitForURLOptions{
		Timeout: playwright.Float(float64(cfg.Timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("failed to navigate to '%s': %w", successURL, err)
	}
	isAuthorized, err := hasUserStatusDropdown(page, cfg.Timeout)
	if err != nil {
		return fmt.Errorf("could not validate login status after authentication: %w", err)
	}
	if !isAuthorized {
		return fmt.Errorf("authentication failed: could not confirm login after the sign-in flow; the sign-in URL might be expired, request a new link and try again")
	}

	time.Sleep(5 * time.Second)
	return nil
}
