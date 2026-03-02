package wolt

import "time"

func ValidateAuthURL(raw string) (string, error) {
	return validateAuthURL(raw)
}

func RunAuth(cfg Config, eraseData bool, authURL string) error {
	return runAuth(cfg, eraseData, authURL)
}

func RunSearch(cfg Config, query string) error {
	return runSearch(cfg, query)
}

func RunBasket(cfg Config) error {
	return runBasket(cfg)
}

func RunBasketAdd(cfg Config, venueSlug, itemID string) error {
	return runBasketAdd(cfg, venueSlug, itemID)
}

func RunBasketRemove(cfg Config, venueSlug, itemID string) error {
	return runBasketRemove(cfg, venueSlug, itemID)
}

func RunCheckout(cfg Config, venueSlug string) error {
	return runCheckout(cfg, venueSlug)
}

func ExtractSearchProducts(responseJSON interface{}) []SearchProduct {
	return extractSearchProducts(responseJSON)
}

func BuildBasketAddURL(cfg Config, venueSlug, itemID string) string {
	return buildBasketAddURL(cfg, venueSlug, itemID)
}

func BuildCheckoutURL(cfg Config, venueSlug string) string {
	return buildCheckoutURL(cfg, venueSlug)
}

func BuildCheckoutCartItemSelector(itemID string) string {
	return buildCheckoutCartItemSelector(itemID)
}

func IsBasketPageRequest(method, requestURL string) bool {
	return isBasketPageRequest(method, requestURL)
}

func BasketRestoreModalWaitTimeout(timeout time.Duration) time.Duration {
	return basketRestoreModalWaitTimeout(timeout)
}

func BasketCheckoutCartItemWaitTimeout(timeout time.Duration) time.Duration {
	return basketCheckoutCartItemWaitTimeout(timeout)
}

func UserStatusDropdownWaitTimeout(timeout time.Duration) time.Duration {
	return userStatusDropdownWaitTimeout(timeout)
}

func ExtractBasketOutputs(responseJSON interface{}) []BasketOutput {
	return extractBasketOutputs(responseJSON)
}

func BasketContainsVenueItem(baskets []BasketOutput, venueSlug, itemID string) bool {
	return basketContainsVenueItem(baskets, venueSlug, itemID)
}

func BasketItemQuantityForVenue(baskets []BasketOutput, venueSlug, itemID string) int {
	return basketItemQuantityForVenue(baskets, venueSlug, itemID)
}

func IsRetryableRestoreModalClickError(err error) bool {
	return isRetryableRestoreModalClickError(err)
}

func IsPlaywrightTimeoutError(err error) bool {
	return isPlaywrightTimeoutError(err)
}
