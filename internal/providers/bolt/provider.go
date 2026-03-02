package bolt

import (
	"errors"
	"fmt"

	"foodcli/internal/core"
)

var errProviderNotImplemented = errors.New("provider integration is not implemented yet")

type Provider struct{}

func (Provider) Name() string {
	return "bolt"
}

func (Provider) Auth(cfg core.Config, eraseData bool) error {
	return newStubError("auth")
}

func (Provider) Search(cfg core.Config, query string) error {
	return newStubError("search")
}

func (Provider) Basket(cfg core.Config) error {
	return newStubError("basket")
}

func (Provider) BasketAdd(cfg core.Config, venueSlug, itemID string) error {
	return newStubError("basket add")
}

func (Provider) BasketRemove(cfg core.Config, venueSlug, itemID string) error {
	return newStubError("basket remove")
}

func (Provider) Checkout(cfg core.Config, venueSlug string) error {
	return newStubError("checkout")
}

func newStubError(command string) error {
	return fmt.Errorf("provider %q does not support %q yet: %w", "bolt", command, errProviderNotImplemented)
}
