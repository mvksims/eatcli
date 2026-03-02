package providers

import (
	"fmt"
	"strings"

	"foodcli/internal/core"
	"foodcli/internal/providers/bolt"
	"foodcli/internal/providers/wolt"
)

const (
	ProviderWolt = "wolt"
	ProviderBolt = "bolt"
)

type DeliveryProvider interface {
	Name() string
	Auth(cfg core.Config, eraseData bool) error
	Search(cfg core.Config, query string) error
	Basket(cfg core.Config) error
	BasketAdd(cfg core.Config, venueSlug, itemID string) error
	BasketRemove(cfg core.Config, venueSlug, itemID string) error
	Checkout(cfg core.Config, venueSlug string) error
}

func ResolveProviderName(raw string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	if normalized == "" {
		return ProviderWolt, nil
	}

	switch normalized {
	case ProviderWolt, ProviderBolt:
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported provider %q (supported: %s, %s)", raw, ProviderWolt, ProviderBolt)
	}
}

func New(name string) (DeliveryProvider, error) {
	switch name {
	case ProviderWolt:
		return wolt.Provider{}, nil
	case ProviderBolt:
		return bolt.Provider{}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", name)
	}
}
