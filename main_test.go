package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	content := `
success_url_pattern: "https://example.com/dashboard"
success_selector: "#user-avatar"
user_data_dir: "./test_profile"
headless: true
timeout_seconds: 30
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write temp config file: %v", err)
	}

	// Call loadConfig
	cfg, err := loadConfig(configFile)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	// Assert the results
	if cfg.SuccessURLPattern != "https://example.com/dashboard" {
		t.Errorf("expected SuccessURLPattern to be 'https://example.com/dashboard', got '%s'", cfg.SuccessURLPattern)
	}
	if cfg.SuccessSelector != "#user-avatar" {
		t.Errorf("expected SuccessSelector to be '#user-avatar', got '%s'", cfg.SuccessSelector)
	}
	if !filepath.IsAbs(cfg.UserDataDir) {
		t.Errorf("expected UserDataDir to be an absolute path, got '%s'", cfg.UserDataDir)
	}
	if filepath.Base(cfg.UserDataDir) != "test_profile" {
		t.Errorf("expected UserDataDir to end with 'test_profile', got '%s'", cfg.UserDataDir)
	}
	if !cfg.Headless {
		t.Errorf("expected Headless to be true, got false")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout to be 30s, got %v", cfg.Timeout)
	}
}

func TestRunAutomation_Success(t *testing.T) {
	// We need to install playwright browsers for the test
	if err := playwright.Install(&playwright.RunOptions{}); err != nil {
		t.Fatalf("could not install playwright: %v", err)
	}

	// Create a fake server that immediately serves the "logged in" page
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body><div data-test-id="user-avatar"></div></body></html>`))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "profile")
	// Create the UserDataDir so the os.Stat check passes
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		t.Fatalf("Failed to create temp user data dir: %v", err)
	}

	cfg := Config{
		SuccessURLPattern: server.URL,
		SuccessSelector:   "[data-test-id='user-avatar']",
		UserDataDir:       userDataDir,
		Headless:          true,
		Timeout:           15 * time.Second, // Short timeout for test
	}

	err := runAutomation(cfg)
	if err != nil {
		t.Errorf("runAutomation failed unexpectedly: %v", err)
	}
}

func TestRunAutomation_Failure_NoProfile(t *testing.T) {
	tmpDir := t.TempDir()
	userDataDir := filepath.Join(tmpDir, "non_existent_profile")

	cfg := Config{
		SuccessURLPattern: "http://localhost:1234", // Placeholder, URL doesn't matter here
		UserDataDir:       userDataDir,
		Headless:          true,
		Timeout:           5 * time.Second,
	}

	err := runAutomation(cfg)
	if err == nil {
		t.Errorf("runAutomation was expected to fail but did not")
	}

	expectedError := "no saved session found"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("expected error to contain '%s', but got '%s'", expectedError, err.Error())
	}
}

func TestExtractSearchProducts_BasicFields(t *testing.T) {
	response := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"value": map[string]interface{}{
							"id":    "3135258a5f2ffa0c518ab4b8",
							"name":  "Selga biscuits condensed milk 180g",
							"price": float64(289),
							"venue": map[string]interface{}{
								"id":   "62430901d7678f5b344972e4",
								"slug": "wolt-market-grizinkalna",
							},
						},
					},
				},
			},
		},
	}

	products := extractSearchProducts(response)
	if len(products) != 1 {
		t.Fatalf("expected 1 product, got %d", len(products))
	}

	got := products[0]
	if got.ID != "3135258a5f2ffa0c518ab4b8" {
		t.Fatalf("unexpected id: %s", got.ID)
	}
	if got.Name != "Selga biscuits condensed milk 180g" {
		t.Fatalf("unexpected name: %s", got.Name)
	}
	if got.VenueID != "62430901d7678f5b344972e4" {
		t.Fatalf("unexpected venue id: %s", got.VenueID)
	}
	if got.VenueSlug != "wolt-market-grizinkalna" {
		t.Fatalf("unexpected venue slug: %s", got.VenueSlug)
	}
	if got.Price != float64(289) {
		t.Fatalf("unexpected price: %#v", got.Price)
	}
}

func TestExtractSearchProducts_FallbackFromURL(t *testing.T) {
	response := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"value": map[string]interface{}{
							"title": "Vafeļu torte piparkūku 300g Selga",
							"venue": map[string]interface{}{
								"id": "62430901d7678f5b344972e4",
							},
							"pricing": map[string]interface{}{
								"price": map[string]interface{}{
									"amount": float64(399),
								},
							},
							"link": "/en/lva/riga/venue/wolt-market-grizinkalna/selga-cepumi-itemid-3135258a5f2ffa0c518ab4b8",
						},
					},
				},
			},
		},
	}

	products := extractSearchProducts(response)
	if len(products) != 1 {
		t.Fatalf("expected 1 product, got %d", len(products))
	}

	got := products[0]
	if got.ID != "3135258a5f2ffa0c518ab4b8" {
		t.Fatalf("expected id from URL fallback, got %s", got.ID)
	}
	if got.VenueSlug != "wolt-market-grizinkalna" {
		t.Fatalf("expected venue slug from URL fallback, got %s", got.VenueSlug)
	}
	if got.Price != float64(399) {
		t.Fatalf("expected normalized price amount, got %#v", got.Price)
	}
}

func TestExtractSearchProducts_MissingIDStillReturnsProduct(t *testing.T) {
	response := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"value": map[string]interface{}{
							"title": map[string]interface{}{
								"text": "Selga cream biscuits",
							},
							"price": float64(129),
							"venue": map[string]interface{}{
								"slug": "wolt-market-grizinkalna",
							},
						},
					},
				},
			},
		},
	}

	products := extractSearchProducts(response)
	if len(products) != 1 {
		t.Fatalf("expected 1 product, got %d", len(products))
	}

	got := products[0]
	if got.ID == "" {
		t.Fatalf("expected fallback id to be generated")
	}
	if got.Name != "Selga cream biscuits" {
		t.Fatalf("unexpected name: %s", got.Name)
	}
	if got.VenueSlug != "wolt-market-grizinkalna" {
		t.Fatalf("unexpected venue slug: %s", got.VenueSlug)
	}
}

func TestExtractSearchProducts_RealPayloadMenuItemDetails(t *testing.T) {
	response := map[string]interface{}{
		"sections": []interface{}{
			map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"template": "search-menu-item",
						"title":    "Selga šokolādes glazūrā 190g cepumi",
						"menu_item": map[string]interface{}{
							"id":       "a22bc220dd44c8f8daa8ef96",
							"name":     "Selga šokolādes glazūrā 190g cepumi",
							"price":    float64(289),
							"venue_id": "62430901d7678f5b344972e4",
						},
						"link": map[string]interface{}{
							"type": "menu-item-details",
							"menu_item_details": map[string]interface{}{
								"id":         "a22bc220dd44c8f8daa8ef96",
								"price":      float64(289),
								"venue_id":   "62430901d7678f5b344972e4",
								"venue_slug": "wolt-market-grizinkalna",
								"name":       "Selga šokolādes glazūrā 190g cepumi",
							},
						},
					},
				},
			},
		},
	}

	products := extractSearchProducts(response)
	if len(products) == 0 {
		t.Fatalf("expected products from payload, got zero")
	}

	var matched *SearchProduct
	for i := range products {
		if products[i].Name == "Selga šokolādes glazūrā 190g cepumi" {
			matched = &products[i]
			break
		}
	}

	if matched == nil {
		t.Fatalf("expected to find known fixture product by name")
	}

	if matched.ID == "" || strings.HasPrefix(matched.ID, "section_") {
		t.Fatalf("expected real product id from payload, got %q", matched.ID)
	}
	if matched.ID != "a22bc220dd44c8f8daa8ef96" {
		t.Fatalf("unexpected product id: %s", matched.ID)
	}
	if matched.Price != float64(289) {
		t.Fatalf("expected price 289, got %#v", matched.Price)
	}
	if matched.VenueID != "62430901d7678f5b344972e4" {
		t.Fatalf("unexpected venue id: %s", matched.VenueID)
	}
	if matched.VenueSlug != "wolt-market-grizinkalna" {
		t.Fatalf("unexpected venue slug: %s", matched.VenueSlug)
	}
}

func TestBuildBasketAddURL(t *testing.T) {
	got := buildBasketAddURL("wolt-market-grizinkalna", "3135258a5f2ffa0c518ab4b8")
	want := "https://wolt.com/en/lva/riga/venue/wolt-market-grizinkalna/itemid-3135258a5f2ffa0c518ab4b8"
	if got != want {
		t.Fatalf("unexpected basket add URL: got %q want %q", got, want)
	}
}

func TestIsBasketPageRequest(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		url       string
		wantMatch bool
	}{
		{
			name:      "matches GET baskets endpoint",
			method:    "GET",
			url:       "https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets?currency=EUR",
			wantMatch: true,
		},
		{
			name:      "matches GET case-insensitive method",
			method:    "get",
			url:       "https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets",
			wantMatch: true,
		},
		{
			name:      "does not match non-GET method",
			method:    "POST",
			url:       "https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets",
			wantMatch: false,
		},
		{
			name:      "does not match unrelated URL",
			method:    "GET",
			url:       "https://consumer-api.wolt.com/order-xp/web/v1/pages/search",
			wantMatch: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := isBasketPageRequest(tc.method, tc.url)
			if got != tc.wantMatch {
				t.Fatalf("unexpected match: got %v want %v", got, tc.wantMatch)
			}
		})
	}
}

func TestBasketRestoreModalWaitTimeout(t *testing.T) {
	tests := []struct {
		name    string
		input   time.Duration
		wantOut time.Duration
	}{
		{
			name:    "uses max wait for zero timeout",
			input:   0,
			wantOut: 5 * time.Second,
		},
		{
			name:    "uses provided timeout when shorter",
			input:   2 * time.Second,
			wantOut: 2 * time.Second,
		},
		{
			name:    "caps long timeout",
			input:   30 * time.Second,
			wantOut: 5 * time.Second,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := basketRestoreModalWaitTimeout(tc.input)
			if got != tc.wantOut {
				t.Fatalf("unexpected timeout clamp: got %v want %v", got, tc.wantOut)
			}
		})
	}
}
