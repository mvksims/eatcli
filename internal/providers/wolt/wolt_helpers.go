package wolt

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

func closeSession(pw *playwright.Playwright, ctx playwright.BrowserContext) {
	if ctx != nil {
		_ = ctx.Close()
	}
	if pw != nil {
		_ = pw.Stop()
	}
}

func closeSessionWithWarnings(pw *playwright.Playwright, ctx playwright.BrowserContext) {
	if ctx != nil {
		if err := ctx.Close(); err != nil {
			log.Printf("Warning: Could not close browser context: %v", err)
		}
	}
	if pw != nil {
		if err := pw.Stop(); err != nil {
			log.Printf("Warning: Could not stop playwright: %v", err)
		}
	}
}

func launchInteractiveSession(cfg Config) (*playwright.Playwright, playwright.BrowserContext, playwright.Page, error) {
	return launchPersistentSession(cfg, browserSessionOptions{
		headless:         false,
		ensureProfileDir: true,
	})
}

func waitForPageLoadState(page playwright.Page, state *playwright.LoadState, timeout time.Duration) error {
	return page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   state,
		Timeout: playwright.Float(float64(timeout.Milliseconds())),
	})
}

func gotoAndWaitForPageLoad(page playwright.Page, targetURL string, timeout time.Duration, gotoErrPrefix, loadErrPrefix string) error {
	if _, err := page.Goto(targetURL); err != nil {
		return fmt.Errorf("%s: %w", gotoErrPrefix, err)
	}

	if err := waitForPageLoadState(page, playwright.LoadStateLoad, timeout); err != nil {
		return fmt.Errorf("%s: %w", loadErrPrefix, err)
	}

	return nil
}

func gotoWaitLoadAndEnsureUserAuthorized(page playwright.Page, targetURL string, timeout time.Duration, gotoErrPrefix, loadErrPrefix string) error {
	if err := gotoAndWaitForPageLoad(page, targetURL, timeout, gotoErrPrefix, loadErrPrefix); err != nil {
		return err
	}
	return ensureUserAuthorized(page, timeout)
}

func basketCaptureTargetURL(cfg Config) string {
	if cfg.SuccessURLPattern != "" {
		return cfg.SuccessURLPattern
	}
	return defaultBasketCaptureURL
}

func clickFirstLocatorWithRetry(page playwright.Page, selector string, attempts int, timeout time.Duration, onRetry func(int, error)) error {
	if attempts <= 0 {
		attempts = 1
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		locator := page.Locator(selector).Nth(0)
		err := locator.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(float64(timeout.Milliseconds())),
		})
		if err == nil {
			return nil
		}

		lastErr = err
		if attempt == attempts {
			break
		}
		if isRetryableRestoreModalClickError(err) || isPlaywrightTimeoutError(err) {
			if onRetry != nil {
				onRetry(attempt, err)
			}
			time.Sleep(150 * time.Millisecond)
			continue
		}
		return err
	}

	return lastErr
}

func wrapClickError(message string, err error) error {
	if err == nil {
		return nil
	}
	if isRetryableRestoreModalClickError(err) || isPlaywrightTimeoutError(err) {
		return fmt.Errorf("%s after retries: %w", message, err)
	}
	return fmt.Errorf("%s: %w", message, err)
}

func waitForVisibleLocator(locator playwright.Locator, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return locator.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(timeout.Milliseconds())),
	})
}

func waitForFirstVisibleLocator(page playwright.Page, selector string, timeout time.Duration) (playwright.Locator, error) {
	locator := page.Locator(selector).Nth(0)
	if err := waitForVisibleLocator(locator, timeout); err != nil {
		return nil, err
	}
	return locator, nil
}

func waitForFirstVisibleLocatorWithMessage(page playwright.Page, selector string, timeout time.Duration, message string) (playwright.Locator, error) {
	locator, err := waitForFirstVisibleLocator(page, selector, timeout)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", message, err)
	}
	return locator, nil
}

func clickLocatorWithMessage(locator playwright.Locator, message string) error {
	if err := locator.Click(); err != nil {
		return fmt.Errorf("%s: %w", message, err)
	}
	return nil
}

func openBasketCheckoutPage(cfg Config, venueSlug, flowLabel string) (*playwright.Playwright, playwright.BrowserContext, playwright.Page, <-chan basketResponseEvent, error) {
	targetURL := buildCheckoutURL(cfg, venueSlug)
	fmt.Printf("Opening checkout page for basket %s: %s\n", flowLabel, targetURL)

	pw, ctx, page, err := launchInteractiveSession(cfg)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	resChan := attachBasketResponseCapture(page)

	if err := gotoWaitLoadAndEnsureUserAuthorized(
		page,
		targetURL,
		cfg.Timeout,
		"could not go to checkout URL",
		"checkout page did not fully load",
	); err != nil {
		closeSession(pw, ctx)
		return nil, nil, nil, nil, err
	}

	return pw, ctx, page, resChan, nil
}

func ensureCheckoutCartItemPresent(page playwright.Page, itemID string, timeout time.Duration) (string, error) {
	cartItemSelector := buildCheckoutCartItemSelector(itemID)
	hasCartItem, err := waitForCheckoutCartItem(page, cartItemSelector, basketCheckoutCartItemWaitTimeout(timeout))
	if err != nil {
		return "", err
	}
	if !hasCartItem {
		return "", fmt.Errorf("item '%s' was found in basket pre-check but not found on checkout page", itemID)
	}
	return cartItemSelector, nil
}

func openCheckoutCartItemModal(page playwright.Page, cartItemSelector, itemID string, timeout time.Duration) error {
	cartItemButton := page.Locator(cartItemSelector).Nth(0).Locator("button").Nth(0)

	if err := waitForVisibleLocator(cartItemButton, timeout); err != nil {
		return fmt.Errorf("could not find cart item button for '%s': %w", itemID, err)
	}

	if err := clickLocatorWithMessage(cartItemButton, fmt.Sprintf("could not click cart item button for '%s'", itemID)); err != nil {
		return err
	}
	return nil
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

func ensureUserAuthorized(page playwright.Page, timeout time.Duration) error {
	visible, err := hasUserStatusDropdown(page, timeout)
	if err != nil {
		return err
	}
	if !visible {
		return fmt.Errorf("session appears logged out. Please authorize with the 'auth' command first")
	}
	return nil
}

func hasUserStatusDropdown(page playwright.Page, timeout time.Duration) (bool, error) {
	waitTimeout := userStatusDropdownWaitTimeout(timeout)
	visible, err := isLocatorVisible(page, userStatusDropdownID, waitTimeout)
	if err != nil {
		return false, fmt.Errorf("could not verify login status via '%s': %w", userStatusDropdownID, err)
	}
	return visible, nil
}

func isLocatorVisible(page playwright.Page, selector string, timeout time.Duration) (bool, error) {
	locator := page.Locator(selector).Nth(0)
	if err := waitForVisibleLocator(locator, timeout); err != nil {
		if isPlaywrightTimeoutError(err) {
			return false, nil
		}
		return false, fmt.Errorf("could not inspect locator '%s': %w", selector, err)
	}

	return true, nil
}

func userStatusDropdownWaitTimeout(timeout time.Duration) time.Duration {
	const maxWait = 10 * time.Second
	if timeout <= 0 {
		return maxWait
	}
	if timeout < maxWait {
		return timeout
	}
	return maxWait
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

func buildBasketAddURL(cfg Config, venueSlug, itemID string) string {
	return fmt.Sprintf(
		"%s/venue/%s/itemid-%s",
		cfg.VenueBaseURL,
		url.PathEscape(venueSlug),
		url.PathEscape(itemID),
	)
}

func buildCheckoutURL(cfg Config, venueSlug string) string {
	return fmt.Sprintf(
		"%s/venue/%s/checkout",
		cfg.VenueBaseURL,
		url.PathEscape(venueSlug),
	)
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
