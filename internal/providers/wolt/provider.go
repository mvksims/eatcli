package wolt

import (
	"fmt"
	"os"
)

type Provider struct{}

func (Provider) Name() string {
	return "wolt"
}

func (Provider) Auth(cfg Config, eraseData bool) error {
	fmt.Fprint(os.Stderr, "Go to wolt.com and try to login with your email address. Then copy the sign-in URL from the received email into here: ")
	var authURL string
	if _, err := fmt.Scanln(&authURL); err != nil {
		return fmt.Errorf("failed to read URL: %w", err)
	}

	validatedAuthURL, err := validateAuthURL(authURL)
	if err != nil {
		return err
	}

	return runAuth(cfg, eraseData, validatedAuthURL)
}

func (Provider) Search(cfg Config, query string) error {
	return runSearch(cfg, query)
}

func (Provider) Basket(cfg Config) error {
	return runBasket(cfg)
}

func (Provider) BasketAdd(cfg Config, venueSlug, itemID string) error {
	return runBasketAdd(cfg, venueSlug, itemID)
}

func (Provider) BasketRemove(cfg Config, venueSlug, itemID string) error {
	return runBasketRemove(cfg, venueSlug, itemID)
}

func (Provider) Checkout(cfg Config, venueSlug string) error {
	return runCheckout(cfg, venueSlug)
}
