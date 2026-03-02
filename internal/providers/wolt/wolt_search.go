package wolt

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

func runSearch(cfg Config, query string) error {
	pw, ctx, page, err := launchPersistentSession(cfg, browserSessionOptions{
		headless:               cfg.Headless,
		requireExistingProfile: true,
	})
	if err != nil {
		return err
	}
	defer closeSession(pw, ctx)

	searchURL := fmt.Sprintf("https://wolt.com/en/search?q=%s&target=items&filters=delivers_now%%3Ddelivers_now_toggle", url.QueryEscape(query))

	fmt.Printf("Searching for: %s\n", query)

	var body []byte

	// Use a channel to capture the right response to avoid deadlock and handle multiple requests
	type searchRes struct {
		body []byte
		err  error
	}
	resChan := make(chan searchRes, 1)

	page.OnResponse(func(res playwright.Response) {
		if strings.Contains(res.URL(), "https://restaurant-api.wolt.com/v1/pages/search") {
			if res.Request().Method() == "OPTIONS" {
				return
			}
			// Process in a goroutine to avoid deadlocking the event loop
			go func() {
				b, err := res.Body()
				select {
				case resChan <- searchRes{body: b, err: err}:
				default:
					// Already captured a response
				}
			}()
		}
	})

	if _, err = page.Goto(searchURL); err != nil {
		return fmt.Errorf("could not go to search URL: %w", err)
	}

	select {
	case res := <-resChan:
		if res.err != nil {
			return fmt.Errorf("could not read response body: %w", res.err)
		}
		body = res.body
	case <-time.After(cfg.Timeout):
		return fmt.Errorf("timed out waiting for API response from https://restaurant-api.wolt.com/v1/pages/search")
	}

	var responseJSON interface{}
	if err := json.Unmarshal(body, &responseJSON); err != nil {
		return fmt.Errorf("could not unmarshal response JSON: %w. Body length: %d", err, len(body))
	}

	products := extractSearchProducts(responseJSON)

	result := map[string]interface{}{
		"keyword":  query,
		"count":    len(products),
		"products": products,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))

	return nil
}

type SearchProduct struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Price     interface{} `json:"price"`
	VenueID   string      `json:"venue_id"`
	VenueSlug string      `json:"venue_slug"`
}

func extractSearchProducts(responseJSON interface{}) []SearchProduct {
	resMap, ok := responseJSON.(map[string]interface{})
	if !ok {
		return nil
	}

	sections, ok := resMap["sections"].([]interface{})
	if !ok {
		if nestedSections := findNestedArrayByKey(resMap, "sections"); nestedSections != nil {
			sections = nestedSections
		} else {
			return nil
		}
	}

	products := make([]SearchProduct, 0)
	seen := make(map[string]struct{})

	for sectionIdx, sectionRaw := range sections {
		section, ok := sectionRaw.(map[string]interface{})
		if !ok {
			continue
		}
		items, ok := section["items"].([]interface{})
		if !ok {
			continue
		}

		for itemIdx, itemRaw := range items {
			item, ok := itemRaw.(map[string]interface{})
			if !ok {
				continue
			}

			product, ok := extractSearchProduct(item, sectionIdx, itemIdx)
			if !ok {
				continue
			}

			key := product.ID + "|" + product.VenueID + "|" + product.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			products = append(products, product)
		}
	}

	return products
}

func extractSearchProduct(item map[string]interface{}, sectionIdx, itemIdx int) (SearchProduct, bool) {
	scopes := buildProductScopes(item)

	id := firstStringFromScopes(scopes, []string{"id", "item_id", "product_id", "public_id", "reference_id"})
	name := firstDisplayStringFromScopes(scopes, []string{"name", "title", "item_name", "display_name", "product_name"})
	price, hasPrice := firstPriceFromScopes(scopes)

	venueID := ""
	venueSlug := ""

	for _, scope := range scopes {
		if venueID == "" {
			venueID = firstStringFromScope(scope, []string{"venue_id"})
		}
		if venueSlug == "" {
			venueSlug = firstStringFromScope(scope, []string{"venue_slug"})
		}
		if venueID != "" && venueSlug != "" {
			break
		}
	}

	if venue := findNestedMapByKey(item, "venue"); venue != nil {
		if venueID == "" {
			venueID = firstStringFromScope(venue, []string{"id", "venue_id"})
		}
		if venueSlug == "" {
			venueSlug = firstStringFromScope(venue, []string{"slug", "venue_slug"})
		}

		if nestedVenue := asMap(venue["value"]); nestedVenue != nil {
			if venueID == "" {
				venueID = firstStringFromScope(nestedVenue, []string{"id", "venue_id"})
			}
			if venueSlug == "" {
				venueSlug = firstStringFromScope(nestedVenue, []string{"slug", "venue_slug"})
			}
		}
	}

	textValues := collectStringValues(item)
	if name == "" {
		name = fallbackNameFromTextValues(textValues)
	}
	for _, text := range textValues {
		if id == "" {
			id = extractIDFromText(text)
		}
		if venueSlug == "" {
			venueSlug = extractVenueSlugFromText(text)
		}
		if id != "" && venueSlug != "" {
			break
		}
	}

	if name == "" {
		return SearchProduct{}, false
	}
	if id == "" {
		id = fmt.Sprintf("section_%d_item_%d", sectionIdx, itemIdx)
	}

	var normalizedPrice interface{}
	if hasPrice {
		normalizedPrice = normalizePrice(price)
	}

	return SearchProduct{
		ID:        id,
		Name:      name,
		Price:     normalizedPrice,
		VenueID:   venueID,
		VenueSlug: venueSlug,
	}, true
}

func buildProductScopes(item map[string]interface{}) []map[string]interface{} {
	scopes := make([]map[string]interface{}, 0, 8)

	for _, key := range []string{"value", "item", "product", "content", "data", "details", "menu_item", "link", "menu_item_details"} {
		if scope := asMap(item[key]); scope != nil {
			scopes = append(scopes, scope)
		}
	}

	if value := asMap(item["value"]); value != nil {
		for _, key := range []string{"item", "product", "content", "menu_item", "link", "menu_item_details"} {
			if scope := asMap(value[key]); scope != nil {
				scopes = append(scopes, scope)
			}
		}
	}

	if link := asMap(item["link"]); link != nil {
		for _, key := range []string{"menu_item_details", "action_link"} {
			if scope := asMap(link[key]); scope != nil {
				scopes = append(scopes, scope)
			}
		}
	}

	scopes = append(scopes, item)
	return scopes
}

func firstPriceFromScopes(scopes []map[string]interface{}) (interface{}, bool) {
	for _, scope := range scopes {
		if raw, ok := firstAnyFromScope(scope, []string{
			"price",
			"baseprice",
			"base_price",
			"current_price",
			"display_price",
			"formatted_price",
		}); ok {
			return raw, true
		}

		if pricing, ok := scope["pricing"]; ok {
			if value, ok := extractPriceFromValue(pricing); ok {
				return value, true
			}
		}
	}
	return nil, false
}
