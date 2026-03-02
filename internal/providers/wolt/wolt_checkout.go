package wolt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

func runCheckout(cfg Config, venueSlug string) error {
	targetURL := buildCheckoutURL(cfg, venueSlug)
	fmt.Printf("Opening checkout page: %s\n", targetURL)

	pw, ctx, page, err := launchInteractiveSession(cfg)
	if err != nil {
		return err
	}
	defer closeSession(pw, ctx)

	if err := gotoWaitLoadAndEnsureUserAuthorized(
		page,
		targetURL,
		cfg.Timeout,
		"could not go to checkout URL",
		"checkout page did not fully load",
	); err != nil {
		return err
	}

	sendOrderButtonSelector := `[data-test-id="SendOrderButton"]`
	sendOrderButton, err := waitForFirstVisibleLocatorWithMessage(
		page,
		sendOrderButtonSelector,
		cfg.Timeout,
		fmt.Sprintf("could not find Send Order button '%s'", sendOrderButtonSelector),
	)
	if err != nil {
		return err
	}

	if err := clickLocatorWithMessage(sendOrderButton, fmt.Sprintf("could not click Send Order button '%s'", sendOrderButtonSelector)); err != nil {
		return err
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
