package wolt

import (
	"fmt"
	"os"

	"foodcli/internal/core"
	"github.com/playwright-community/playwright-go"
)

const (
	defaultBasketCaptureURL = "https://wolt.com/en/discovery"
	basketAPIURL            = "https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets"
	userStatusDropdownID    = `[data-test-id="UserStatusDropdown"]`
	defaultViewportWidth    = 1440
	defaultViewportHeight   = 810
	firefoxUserAgent        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:126.0) Gecko/20100101 Firefox/126.0"
)

const antiDetectionInitScript = `
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

type browserSessionOptions struct {
	headless               bool
	requireExistingProfile bool
	ensureProfileDir       bool
}

func ensureUserDataDirExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no saved session found in '%s'. Please run the 'auth' command first", path)
		}
		return fmt.Errorf("could not inspect user data directory '%s': %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("user data directory '%s' exists but is not a directory", path)
	}
	return nil
}

func validateEraseUserDataDir(path string) error {
	return core.ValidateEraseUserDataDir(path)
}

func launchPersistentSession(cfg Config, opts browserSessionOptions) (*playwright.Playwright, playwright.BrowserContext, playwright.Page, error) {
	if opts.requireExistingProfile {
		if err := ensureUserDataDirExists(cfg.UserDataDir); err != nil {
			return nil, nil, nil, err
		}
	}
	if opts.ensureProfileDir {
		if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
			return nil, nil, nil, fmt.Errorf("could not create user data directory: %w", err)
		}
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not start playwright: %w", err)
	}

	ctx, err := pw.Firefox.LaunchPersistentContext(cfg.UserDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:  playwright.Bool(opts.headless),
		UserAgent: playwright.String(firefoxUserAgent),
	})
	if err != nil {
		_ = pw.Stop()
		return nil, nil, nil, fmt.Errorf("could not launch persistent context: %w", err)
	}

	cleanup := func() {
		_ = ctx.Close()
		_ = pw.Stop()
	}

	if err := ctx.AddInitScript(playwright.Script{Content: playwright.String(antiDetectionInitScript)}); err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("could not add init script: %w", err)
	}

	page, err := ctx.NewPage()
	if err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("could not create new page: %w", err)
	}

	if err := page.SetViewportSize(defaultViewportWidth, defaultViewportHeight); err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("could not set viewport size: %w", err)
	}

	if err := setupHeaderRemoval(page); err != nil {
		cleanup()
		return nil, nil, nil, fmt.Errorf("could not set up header removal: %w", err)
	}

	return pw, ctx, page, nil
}
