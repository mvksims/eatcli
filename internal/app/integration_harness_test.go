//go:build integration

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"foodcli/internal/providers/wolt"
)

const (
	harnessDefaultQueryOne = "milk"
	harnessDefaultQueryTwo = "bread"
)

type harnessSearchResult struct {
	Keyword  string               `json:"keyword"`
	Count    int                  `json:"count"`
	Products []wolt.SearchProduct `json:"products"`
}

type harnessBasketActionResult struct {
	Action    string              `json:"action"`
	VenueSlug string              `json:"venue_slug"`
	ItemID    string              `json:"item_id"`
	Count     int                 `json:"count"`
	Baskets   []wolt.BasketOutput `json:"baskets"`
}

type harnessBasketViewResult struct {
	Count   int                 `json:"count"`
	Baskets []wolt.BasketOutput `json:"baskets"`
}

func TestIntegrationHarness_SearchAddAddRemoveSameRetailer(t *testing.T) {
	if strings.TrimSpace(os.Getenv("EATCLI_E2E")) == "" {
		t.Skip("set EATCLI_E2E=1 to run this integration harness")
	}

	configPath := envOrDefault("EATCLI_E2E_CONFIG", "config.yml")
	queryOne := envOrDefault("EATCLI_E2E_QUERY_ONE", harnessDefaultQueryOne)
	queryTwo := envOrDefault("EATCLI_E2E_QUERY_TWO", harnessDefaultQueryTwo)

	cfg, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config %q: %v", configPath, err)
	}

	searchOne, err := runHarnessSearch(cfg, queryOne)
	if err != nil {
		t.Fatalf("search for query %q failed: %v", queryOne, err)
	}
	searchTwo, err := runHarnessSearch(cfg, queryTwo)
	if err != nil {
		t.Fatalf("search for query %q failed: %v", queryTwo, err)
	}

	productOne, productTwo, ok := pickProductsFromSameVenue(searchOne.Products, searchTwo.Products)
	if !ok {
		t.Skipf("could not find products from the same venue for queries %q and %q", queryOne, queryTwo)
	}
	venueSlug := strings.TrimSpace(productOne.VenueSlug)
	t.Logf("selected venue=%s item1=%s (%s) item2=%s (%s)", venueSlug, productOne.ID, productOne.Name, productTwo.ID, productTwo.Name)

	addOne, err := runHarnessBasketAdd(cfg, venueSlug, productOne.ID)
	if err != nil {
		t.Fatalf("basket add failed for first product: %v", err)
	}
	if addOne.Action != "add" {
		t.Fatalf("unexpected first add action: got %q", addOne.Action)
	}

	addTwo, err := runHarnessBasketAdd(cfg, venueSlug, productTwo.ID)
	if err != nil {
		t.Fatalf("basket add failed for second product: %v", err)
	}
	if addTwo.Action != "add" {
		t.Fatalf("unexpected second add action: got %q", addTwo.Action)
	}

	removed, err := runHarnessBasketRemove(cfg, venueSlug, productOne.ID)
	if err != nil {
		t.Fatalf("basket remove failed for first product: %v", err)
	}
	if removed.Action != "remove" {
		t.Fatalf("unexpected remove action: got %q", removed.Action)
	}

	view, err := runHarnessBasketView(cfg)
	if err != nil {
		t.Fatalf("basket view failed: %v", err)
	}

	remainingFirst := wolt.BasketItemQuantityForVenue(view.Baskets, venueSlug, productOne.ID)
	if remainingFirst != 0 {
		t.Fatalf("expected first product %q to be removed from venue %q, remaining quantity=%d", productOne.ID, venueSlug, remainingFirst)
	}

	remainingSecond := wolt.BasketItemQuantityForVenue(view.Baskets, venueSlug, productTwo.ID)
	if remainingSecond <= 0 {
		t.Fatalf("expected second product %q to remain in venue %q basket, quantity=%d", productTwo.ID, venueSlug, remainingSecond)
	}
}

func runHarnessSearch(cfg Config, query string) (harnessSearchResult, error) {
	var result harnessSearchResult
	output, err := captureCommandOutput(func() error {
		return wolt.RunSearch(cfg, query)
	})
	if err != nil {
		return result, err
	}
	if err := unmarshalLastJSONObject(output, &result); err != nil {
		return result, fmt.Errorf("could not parse search JSON output: %w", err)
	}
	return result, nil
}

func runHarnessBasketAdd(cfg Config, venueSlug, itemID string) (harnessBasketActionResult, error) {
	var result harnessBasketActionResult
	output, err := captureCommandOutput(func() error {
		return wolt.RunBasketAdd(cfg, venueSlug, itemID)
	})
	if err != nil {
		return result, err
	}
	if err := unmarshalLastJSONObject(output, &result); err != nil {
		return result, fmt.Errorf("could not parse basket add JSON output: %w", err)
	}
	return result, nil
}

func runHarnessBasketRemove(cfg Config, venueSlug, itemID string) (harnessBasketActionResult, error) {
	var result harnessBasketActionResult
	output, err := captureCommandOutput(func() error {
		return wolt.RunBasketRemove(cfg, venueSlug, itemID)
	})
	if err != nil {
		return result, err
	}
	if err := unmarshalLastJSONObject(output, &result); err != nil {
		return result, fmt.Errorf("could not parse basket remove JSON output: %w", err)
	}
	return result, nil
}

func runHarnessBasketView(cfg Config) (harnessBasketViewResult, error) {
	var result harnessBasketViewResult
	output, err := captureCommandOutput(func() error {
		return wolt.RunBasket(cfg)
	})
	if err != nil {
		return result, err
	}
	if err := unmarshalLastJSONObject(output, &result); err != nil {
		return result, fmt.Errorf("could not parse basket JSON output: %w", err)
	}
	return result, nil
}

func pickProductsFromSameVenue(first, second []wolt.SearchProduct) (wolt.SearchProduct, wolt.SearchProduct, bool) {
	for _, one := range first {
		oneID := strings.TrimSpace(one.ID)
		oneVenue := strings.TrimSpace(one.VenueSlug)
		if oneID == "" || oneVenue == "" {
			continue
		}

		for _, two := range second {
			twoID := strings.TrimSpace(two.ID)
			twoVenue := strings.TrimSpace(two.VenueSlug)
			if twoID == "" || twoVenue == "" || oneID == twoID {
				continue
			}
			if strings.EqualFold(oneVenue, twoVenue) {
				return one, two, true
			}
		}
	}

	return wolt.SearchProduct{}, wolt.SearchProduct{}, false
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func captureCommandOutput(run func() error) (string, error) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("could not create stdout capture pipe: %w", err)
	}

	os.Stdout = writer
	runErr := run()
	closeErr := writer.Close()
	os.Stdout = originalStdout

	captured, readErr := io.ReadAll(reader)
	_ = reader.Close()

	if readErr != nil {
		return "", fmt.Errorf("could not read captured stdout: %w", readErr)
	}
	if runErr != nil {
		return string(captured), runErr
	}
	if closeErr != nil {
		return string(captured), fmt.Errorf("could not close stdout capture writer: %w", closeErr)
	}

	return string(captured), nil
}

func unmarshalLastJSONObject(output string, target interface{}) error {
	payload, err := extractLastJSONObject(output)
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func extractLastJSONObject(output string) ([]byte, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil, fmt.Errorf("output is empty")
	}

	for start := strings.LastIndex(trimmed, "{"); start >= 0; {
		candidate := trimmed[start:]
		decoder := json.NewDecoder(strings.NewReader(candidate))
		decoder.UseNumber()

		var payload interface{}
		if err := decoder.Decode(&payload); err == nil {
			consumed := int(decoder.InputOffset())
			if strings.TrimSpace(candidate[consumed:]) == "" {
				return []byte(strings.TrimSpace(candidate[:consumed])), nil
			}
		}

		if start == 0 {
			break
		}
		start = strings.LastIndex(trimmed[:start], "{")
	}

	return nil, fmt.Errorf("could not locate trailing JSON object in command output")
}
