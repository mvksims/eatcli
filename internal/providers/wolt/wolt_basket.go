package wolt

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

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

	pw, ctx, page, err := launchPersistentSession(cfg, browserSessionOptions{
		headless:               cfg.Headless,
		requireExistingProfile: true,
	})
	if err != nil {
		return err
	}
	defer closeSession(pw, ctx)

	resChan := attachBasketResponseCapture(page)
	requestStartedAt := time.Now()
	targetURL := basketCaptureTargetURL(cfg)

	if err := gotoWaitLoadAndEnsureUserAuthorized(
		page,
		targetURL,
		cfg.Timeout,
		"could not go to basket page",
		"basket page did not fully load",
	); err != nil {
		return err
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
	itemAlreadyInVenueBasket, err := isBasketItemPresentForVenue(cfg, venueSlug, itemID)
	if err != nil {
		return err
	}

	if itemAlreadyInVenueBasket {
		if err := runBasketAddFromCheckout(cfg, venueSlug, itemID); err != nil {
			return err
		}
		return nil
	}

	if err := runBasketAddFromProductDetail(cfg, venueSlug, itemID); err != nil {
		return err
	}
	return nil
}

func runBasketAddFromProductDetail(cfg Config, venueSlug, itemID string) error {
	targetURL := buildBasketAddURL(cfg, venueSlug, itemID)
	fmt.Printf("Opening basket add page: %s\n", targetURL)

	pw, ctx, page, err := launchInteractiveSession(cfg)
	if err != nil {
		return err
	}
	defer closeSession(pw, ctx)

	resChan := attachBasketResponseCapture(page)

	if err := gotoWaitLoadAndEnsureUserAuthorized(
		page,
		targetURL,
		cfg.Timeout,
		"could not go to basket add URL",
		"basket add page did not fully load",
	); err != nil {
		return err
	}

	_, err = maybeConfirmRestoreOrderModal(page, cfg.Timeout)
	if err != nil {
		return err
	}

	submitSelector := `[data-test-id="product-modal.submit"]`
	if _, err := waitForFirstVisibleLocatorWithMessage(
		page,
		submitSelector,
		cfg.Timeout,
		fmt.Sprintf("could not find submit button '%s'", submitSelector),
	); err != nil {
		return err
	}

	clickStartedAt := time.Now()
	if err := clickFirstLocatorWithRetry(page, submitSelector, 3, 3*time.Second, nil); err != nil {
		return wrapClickError(fmt.Sprintf("could not click submit button '%s'", submitSelector), err)
	}

	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
	if err != nil {
		return err
	}

	printBasketActionResult("add", venueSlug, itemID, basketRes)
	return nil
}

func runBasketRemove(cfg Config, venueSlug, itemID string) error {
	quantity, err := getBasketItemQuantityForVenue(cfg, venueSlug, itemID)
	if err != nil {
		return err
	}
	if quantity <= 0 {
		return fmt.Errorf("item '%s' is not present in basket for venue '%s'", itemID, venueSlug)
	}

	if err := runBasketRemoveFromCheckout(cfg, venueSlug, itemID, quantity); err != nil {
		return err
	}
	return nil
}

func isBasketItemPresentForVenue(cfg Config, venueSlug, itemID string) (bool, error) {
	quantity, err := getBasketItemQuantityForVenue(cfg, venueSlug, itemID)
	if err != nil {
		return false, err
	}
	return quantity > 0, nil
}

func getBasketItemQuantityForVenue(cfg Config, venueSlug, itemID string) (int, error) {
	pw, ctx, page, err := launchPersistentSession(cfg, browserSessionOptions{
		headless:               cfg.Headless,
		requireExistingProfile: true,
	})
	if err != nil {
		return 0, err
	}
	defer closeSession(pw, ctx)

	resChan := attachBasketResponseCapture(page)

	targetURL := basketCaptureTargetURL(cfg)
	requestStartedAt := time.Now()
	if err := gotoAndWaitForPageLoad(
		page,
		targetURL,
		cfg.Timeout,
		"could not go to basket pre-check page",
		"basket pre-check page did not fully load",
	); err != nil {
		return 0, err
	}
	if err := ensureUserAuthorized(page, cfg.Timeout); err != nil {
		return 0, err
	}

	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, requestStartedAt)
	if err != nil {
		return 0, err
	}

	baskets := extractBasketOutputs(basketRes.JSON)
	quantity := basketItemQuantityForVenue(baskets, venueSlug, itemID)
	return quantity, nil
}

func runBasketAddFromCheckout(cfg Config, venueSlug, itemID string) error {
	pw, ctx, page, resChan, err := openBasketCheckoutPage(cfg, venueSlug, "add")
	if err != nil {
		return err
	}
	defer closeSession(pw, ctx)

	cartItemSelector, err := ensureCheckoutCartItemPresent(page, itemID, cfg.Timeout)
	if err != nil {
		return err
	}
	fmt.Printf("Item %s is in basket. Using checkout flow for increment.\n", itemID)

	if err := openCheckoutCartItemModal(page, cartItemSelector, itemID, cfg.Timeout); err != nil {
		return err
	}

	addButtonSelector := `[data-test-id="product-modal.total-price"]`
	addButton, err := waitForFirstVisibleLocatorWithMessage(
		page,
		addButtonSelector,
		cfg.Timeout,
		fmt.Sprintf("could not find basket add button '%s' after cart item selection", addButtonSelector),
	)
	if err != nil {
		return err
	}

	incrementButtonSelector := `[data-test-id="product-modal.quantity.increment"]`
	incrementButton, err := waitForFirstVisibleLocatorWithMessage(
		page,
		incrementButtonSelector,
		cfg.Timeout,
		fmt.Sprintf("could not find quantity increment button '%s' in modal", incrementButtonSelector),
	)
	if err != nil {
		return err
	}

	if err := clickLocatorWithMessage(
		incrementButton,
		fmt.Sprintf("could not click quantity increment button '%s' in modal", incrementButtonSelector),
	); err != nil {
		return err
	}

	clickStartedAt := time.Now()
	if err := clickLocatorWithMessage(addButton, fmt.Sprintf("could not click basket add button '%s'", addButtonSelector)); err != nil {
		return err
	}

	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
	if err != nil {
		return err
	}

	printBasketActionResult("add", venueSlug, itemID, basketRes)
	return nil
}

func runBasketRemoveFromCheckout(cfg Config, venueSlug, itemID string, quantity int) error {
	pw, ctx, page, resChan, err := openBasketCheckoutPage(cfg, venueSlug, "remove")
	if err != nil {
		return err
	}
	defer closeSession(pw, ctx)

	if err := ensureCheckoutReadyForBasketRemove(page, cfg.Timeout); err != nil {
		return err
	}

	cartItemSelector, err := ensureCheckoutCartItemPresent(page, itemID, cfg.Timeout)
	if err != nil {
		return err
	}
	fmt.Printf("Item %s is in basket with quantity %d. Using checkout flow for removal.\n", itemID, quantity)

	if err := openCheckoutCartItemModal(page, cartItemSelector, itemID, cfg.Timeout); err != nil {
		return err
	}

	decrementButtonSelector := `[data-test-id="product-modal.quantity.decrement"]`
	if _, err := waitForFirstVisibleLocatorWithMessage(
		page,
		decrementButtonSelector,
		cfg.Timeout,
		fmt.Sprintf("could not find quantity decrement button '%s' in modal", decrementButtonSelector),
	); err != nil {
		return err
	}

	for i := 0; i < quantity; i++ {
		if err := clickFirstLocatorWithRetry(page, decrementButtonSelector, 3, 3*time.Second, nil); err != nil {
			return wrapClickError(
				fmt.Sprintf("could not click quantity decrement button '%s' in modal on click %d/%d", decrementButtonSelector, i+1, quantity),
				err,
			)
		}
	}

	submitButtonSelector := `[data-test-id="product-modal.submit"]`
	submitButton, err := waitForFirstVisibleLocatorWithMessage(
		page,
		submitButtonSelector,
		cfg.Timeout,
		fmt.Sprintf("could not find modal submit button '%s'", submitButtonSelector),
	)
	if err != nil {
		return err
	}

	clickStartedAt := time.Now()
	if err := clickLocatorWithMessage(submitButton, fmt.Sprintf("could not click modal submit button '%s'", submitButtonSelector)); err != nil {
		return err
	}

	basketRes, err := waitForBasketAPIResponse(resChan, cfg.Timeout, clickStartedAt)
	if err != nil {
		return err
	}

	printBasketActionResult("remove", venueSlug, itemID, basketRes)
	return nil
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
	deadline := time.Now().Add(waitTimeout)
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

func ensureCheckoutReadyForBasketRemove(page playwright.Page, timeout time.Duration) error {
	waitTimeout := basketCheckoutCartItemWaitTimeout(timeout)
	sendOrderButtonSelector := `[data-test-id="SendOrderButton"]`

	sendOrderVisible, err := isLocatorVisible(page, sendOrderButtonSelector, waitTimeout)
	if err != nil {
		return fmt.Errorf("could not inspect checkout readiness via send-order button '%s': %w", sendOrderButtonSelector, err)
	}
	if sendOrderVisible {
		return nil
	}

	_, err = maybeConfirmRestoreOrderModal(page, waitTimeout)
	if err != nil {
		return fmt.Errorf("could not handle optional restore-order modal before checkout recovery: %w", err)
	}

	cartViewButtonSelector := `[data-test-id="cart-view-button"]`
	cartViewVisible, err := isLocatorVisible(page, cartViewButtonSelector, waitTimeout)
	if err != nil {
		return fmt.Errorf("could not inspect cart view button '%s' before checkout recovery: %w", cartViewButtonSelector, err)
	}
	if cartViewVisible {
		if err := clickFirstLocatorWithRetry(page, cartViewButtonSelector, 3, 3*time.Second, nil); err != nil {
			return wrapClickError(fmt.Sprintf("could not click cart view button '%s'", cartViewButtonSelector), err)
		}
	}

	nextStepSelector := `[data-test-id="CartViewNextStepButton"]`
	if _, err := waitForFirstVisibleLocatorWithMessage(
		page,
		nextStepSelector,
		waitTimeout,
		fmt.Sprintf("could not find checkout next-step button '%s'", nextStepSelector),
	); err != nil {
		return err
	}

	if err := clickFirstLocatorWithRetry(page, nextStepSelector, 3, 3*time.Second, nil); err != nil {
		return wrapClickError(fmt.Sprintf("could not click checkout next-step button '%s'", nextStepSelector), err)
	}

	if err := waitForPageLoadState(page, playwright.LoadStateLoad, waitTimeout); err != nil {
		return fmt.Errorf("checkout page did not fully load after clicking '%s': %w", nextStepSelector, err)
	}

	sendOrderVisible, err = isLocatorVisible(page, sendOrderButtonSelector, waitTimeout)
	if err != nil {
		return fmt.Errorf("could not confirm checkout readiness after recovery via send-order button '%s': %w", sendOrderButtonSelector, err)
	}
	if !sendOrderVisible {
		return fmt.Errorf("checkout is not ready for removal: send-order button '%s' is not visible after recovery", sendOrderButtonSelector)
	}
	return nil
}
