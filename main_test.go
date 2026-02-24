package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if cfg.VenueBaseURL != defaultVenueBaseURL {
		t.Errorf("expected VenueBaseURL default to be %q, got %q", defaultVenueBaseURL, cfg.VenueBaseURL)
	}
	if !cfg.Headless {
		t.Errorf("expected Headless to be true, got false")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout to be 30s, got %v", cfg.Timeout)
	}
}

func TestLoadConfig_CustomVenueBaseURL(t *testing.T) {
	content := `
success_url_pattern: "https://example.com/dashboard"
success_selector: "#user-avatar"
user_data_dir: "./test_profile"
venue_base_url: "https://example.com/en/usa/new-york/"
headless: true
timeout_seconds: 30
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write temp config file: %v", err)
	}

	cfg, err := loadConfig(configFile)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}
	if cfg.VenueBaseURL != "https://example.com/en/usa/new-york" {
		t.Fatalf("unexpected VenueBaseURL: %q", cfg.VenueBaseURL)
	}
}

func TestLoadConfig_EmptyUserDataDir(t *testing.T) {
	content := `
success_url_pattern: "https://example.com/dashboard"
success_selector: "#user-avatar"
user_data_dir: ""
headless: true
timeout_seconds: 30
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write temp config file: %v", err)
	}

	_, err := loadConfig(configFile)
	if err == nil {
		t.Fatalf("expected loadConfig to fail for empty user_data_dir")
	}
	if !strings.Contains(err.Error(), "user_data_dir") {
		t.Fatalf("expected user_data_dir validation error, got: %v", err)
	}
}

func TestValidateEraseUserDataDir(t *testing.T) {
	tmpDir := t.TempDir()
	safePath := filepath.Join(tmpDir, "profile", "session-data")
	if err := validateEraseUserDataDir(safePath); err != nil {
		t.Fatalf("expected safe path to pass erase validation, got: %v", err)
	}

	if err := validateEraseUserDataDir(string(filepath.Separator)); err == nil {
		t.Fatalf("expected filesystem root to fail erase validation")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	if err := validateEraseUserDataDir(cwd); err == nil {
		t.Fatalf("expected working directory to fail erase validation")
	}
}

func TestResolveVenueBaseURL(t *testing.T) {
	got, err := resolveVenueBaseURL("")
	if err != nil {
		t.Fatalf("expected default venue base URL, got error: %v", err)
	}
	if got != defaultVenueBaseURL {
		t.Fatalf("unexpected default venue base URL: %q", got)
	}

	got, err = resolveVenueBaseURL("https://example.com/en/usa/new-york/")
	if err != nil {
		t.Fatalf("expected valid custom venue base URL, got error: %v", err)
	}
	if got != "https://example.com/en/usa/new-york" {
		t.Fatalf("unexpected custom venue base URL: %q", got)
	}

	if _, err := resolveVenueBaseURL("example.com/en/usa/new-york"); err == nil {
		t.Fatalf("expected missing scheme/host to fail venue base URL validation")
	}
	if _, err := resolveVenueBaseURL("https://example.com/en/usa/new-york?x=1"); err == nil {
		t.Fatalf("expected query parameters to fail venue base URL validation")
	}
}

func TestValidateAuthURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOut   string
		expectErr bool
	}{
		{
			name:      "accepts wolt domain",
			input:     "https://wolt.com/en/discovery",
			wantOut:   "https://wolt.com/en/discovery",
			expectErr: false,
		},
		{
			name:      "accepts wolt subdomain",
			input:     "https://www.wolt.com/en/discovery",
			wantOut:   "https://www.wolt.com/en/discovery",
			expectErr: false,
		},
		{
			name:      "rejects non wolt domain",
			input:     "https://example.com/login",
			expectErr: true,
		},
		{
			name:      "rejects invalid url",
			input:     "not-a-url",
			expectErr: true,
		},
		{
			name:      "rejects empty input",
			input:     "   ",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateAuthURL(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected validation error for input %q", tc.input)
				}
				if !strings.Contains(err.Error(), "auth URL is incorrect") {
					t.Fatalf("unexpected validation error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected valid URL, got error: %v", err)
			}
			if got != tc.wantOut {
				t.Fatalf("unexpected normalized URL: got %q want %q", got, tc.wantOut)
			}
		})
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
								"slug": "market-grizinkalna",
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
	if got.VenueSlug != "market-grizinkalna" {
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
							"link": "/en/lva/riga/venue/market-grizinkalna/selga-cepumi-itemid-3135258a5f2ffa0c518ab4b8",
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
	if got.VenueSlug != "market-grizinkalna" {
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
								"slug": "market-grizinkalna",
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
	if got.VenueSlug != "market-grizinkalna" {
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
								"venue_slug": "market-grizinkalna",
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
	if matched.VenueSlug != "market-grizinkalna" {
		t.Fatalf("unexpected venue slug: %s", matched.VenueSlug)
	}
}

func TestBuildBasketAddURL(t *testing.T) {
	cfg := Config{VenueBaseURL: "https://example.com/en/lva/riga"}
	got := buildBasketAddURL(cfg, "market-grizinkalna", "3135258a5f2ffa0c518ab4b8")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("buildBasketAddURL returned invalid URL %q: %v", got, err)
	}
	if parsed.Scheme != "https" {
		t.Fatalf("unexpected URL scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		t.Fatalf("expected non-empty host in URL: %q", got)
	}
	wantPath := "/en/lva/riga/venue/market-grizinkalna/itemid-3135258a5f2ffa0c518ab4b8"
	if parsed.Path != wantPath {
		t.Fatalf("unexpected basket add URL path: got %q want %q", parsed.Path, wantPath)
	}
}

func TestBuildCheckoutURL(t *testing.T) {
	cfg := Config{VenueBaseURL: "https://example.com/en/lva/riga"}
	got := buildCheckoutURL(cfg, "market-grizinkalna")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("buildCheckoutURL returned invalid URL %q: %v", got, err)
	}
	if parsed.Scheme != "https" {
		t.Fatalf("unexpected URL scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		t.Fatalf("expected non-empty host in URL: %q", got)
	}
	wantPath := "/en/lva/riga/venue/market-grizinkalna/checkout"
	if parsed.Path != wantPath {
		t.Fatalf("unexpected checkout URL path: got %q want %q", parsed.Path, wantPath)
	}
}

func TestBuildCheckoutCartItemSelector(t *testing.T) {
	got := buildCheckoutCartItemSelector(`item-"abc"\value`)
	want := `div[data-test-id="CartItem"][data-value="item-\"abc\"\\value"]`
	if got != want {
		t.Fatalf("unexpected checkout cart item selector: got %q want %q", got, want)
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
			url:       basketAPIURL + "?currency=EUR",
			wantMatch: true,
		},
		{
			name:      "matches GET case-insensitive method",
			method:    "get",
			url:       basketAPIURL,
			wantMatch: true,
		},
		{
			name:      "does not match non-GET method",
			method:    "POST",
			url:       basketAPIURL,
			wantMatch: false,
		},
		{
			name:      "does not match unrelated URL",
			method:    "GET",
			url:       strings.Replace(basketAPIURL, "/baskets", "/search", 1),
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
			wantOut: 30 * time.Second,
		},
		{
			name:    "uses provided timeout when shorter",
			input:   2 * time.Second,
			wantOut: 2 * time.Second,
		},
		{
			name:    "caps long timeout",
			input:   90 * time.Second,
			wantOut: 30 * time.Second,
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

func TestBasketCheckoutCartItemWaitTimeout(t *testing.T) {
	tests := []struct {
		name    string
		input   time.Duration
		wantOut time.Duration
	}{
		{
			name:    "uses max wait for zero timeout",
			input:   0,
			wantOut: 10 * time.Second,
		},
		{
			name:    "uses provided timeout when shorter",
			input:   3 * time.Second,
			wantOut: 3 * time.Second,
		},
		{
			name:    "caps long timeout",
			input:   90 * time.Second,
			wantOut: 10 * time.Second,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := basketCheckoutCartItemWaitTimeout(tc.input)
			if got != tc.wantOut {
				t.Fatalf("unexpected checkout cart item timeout clamp: got %v want %v", got, tc.wantOut)
			}
		})
	}
}

func TestUserStatusDropdownWaitTimeout(t *testing.T) {
	tests := []struct {
		name    string
		input   time.Duration
		wantOut time.Duration
	}{
		{
			name:    "uses max wait for zero timeout",
			input:   0,
			wantOut: 10 * time.Second,
		},
		{
			name:    "uses provided timeout when shorter",
			input:   2 * time.Second,
			wantOut: 2 * time.Second,
		},
		{
			name:    "caps long timeout",
			input:   45 * time.Second,
			wantOut: 10 * time.Second,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := userStatusDropdownWaitTimeout(tc.input)
			if got != tc.wantOut {
				t.Fatalf("unexpected timeout clamp: got %v want %v", got, tc.wantOut)
			}
		})
	}
}

func TestExtractBasketOutputs_RealPayload(t *testing.T) {
	data, err := os.ReadFile("testdata/baskets-payload.json")
	if err != nil {
		t.Fatalf("failed to read basket payload fixture: %v", err)
	}

	var payload interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("failed to unmarshal basket payload fixture: %v", err)
	}

	baskets := extractBasketOutputs(payload)
	if len(baskets) != 1 {
		t.Fatalf("expected 1 basket, got %d", len(baskets))
	}

	basket := baskets[0]
	if basket.ID != "69939718285af11962aae8b3" {
		t.Fatalf("unexpected basket id: %s", basket.ID)
	}
	if basket.Total != "€8.93" {
		t.Fatalf("unexpected basket total: %#v", basket.Total)
	}
	if basket.VenueSlug != "market-grizinkalna" {
		t.Fatalf("unexpected venue slug: %s", basket.VenueSlug)
	}
	if len(basket.Items) != 4 {
		t.Fatalf("expected 4 basket items, got %d", len(basket.Items))
	}

	firstItem := basket.Items[0]
	if firstItem.ID != "0e51f3f964f104b1f4a91650" {
		t.Fatalf("unexpected first item id: %s", firstItem.ID)
	}
	if firstItem.Count != 1 {
		t.Fatalf("unexpected first item count: %d", firstItem.Count)
	}
	if firstItem.Total != int64(399) {
		t.Fatalf("unexpected first item total: %#v", firstItem.Total)
	}
	if firstItem.ImageURL != "https://imageproxy.example.com/assets/683853f5e3e69835ce1c4105" {
		t.Fatalf("unexpected first item image_url: %s", firstItem.ImageURL)
	}
	if firstItem.Name != "Selga mini vafeles ar vaniļas garšu 250g" {
		t.Fatalf("unexpected first item name: %s", firstItem.Name)
	}
	if !firstItem.IsAvailable {
		t.Fatalf("expected first item to be available")
	}
	if firstItem.Price != float64(399) {
		t.Fatalf("unexpected first item price: %#v", firstItem.Price)
	}

	unavailableItem := basket.Items[2]
	if unavailableItem.ID != "52a7772d08d5b4b5abe2900a" {
		t.Fatalf("unexpected unavailable item id: %s", unavailableItem.ID)
	}
	if unavailableItem.IsAvailable {
		t.Fatalf("expected unavailable item is_available=false")
	}
	if unavailableItem.ImageURL != "" {
		t.Fatalf("expected empty image_url for unavailable fixture item, got %q", unavailableItem.ImageURL)
	}
}

func TestBasketContainsVenueItem(t *testing.T) {
	baskets := []BasketOutput{
		{
			VenueSlug: "market-grizinkalna",
			Items: []BasketItemOutput{
				{ID: "item-a"},
				{ID: "item-b"},
			},
		},
		{
			VenueSlug: "market-agenskalna",
			Items: []BasketItemOutput{
				{ID: "item-a"},
			},
		},
	}

	tests := []struct {
		name      string
		venueSlug string
		itemID    string
		want      bool
	}{
		{
			name:      "matches same venue and item",
			venueSlug: "market-grizinkalna",
			itemID:    "item-a",
			want:      true,
		},
		{
			name:      "does not match different venue",
			venueSlug: "market-grizinkalna",
			itemID:    "missing-item",
			want:      false,
		},
		{
			name:      "does not match item in other venue only",
			venueSlug: "market-agenskalna",
			itemID:    "item-b",
			want:      false,
		},
		{
			name:      "matches case insensitive and trimmed",
			venueSlug: "  MARKET-GRIZINKALNA ",
			itemID:    " ITEM-B ",
			want:      true,
		},
		{
			name:      "returns false on empty inputs",
			venueSlug: "",
			itemID:    "item-a",
			want:      false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := basketContainsVenueItem(baskets, tc.venueSlug, tc.itemID)
			if got != tc.want {
				t.Fatalf("unexpected match result: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestBasketItemQuantityForVenue(t *testing.T) {
	baskets := []BasketOutput{
		{
			VenueSlug: "market-grizinkalna",
			Items: []BasketItemOutput{
				{ID: "item-a", Count: 2},
				{ID: "item-a", Count: 1},
				{ID: "item-b", Count: 4},
			},
		},
		{
			VenueSlug: "market-agenskalna",
			Items: []BasketItemOutput{
				{ID: "item-a", Count: 7},
			},
		},
	}

	tests := []struct {
		name      string
		venueSlug string
		itemID    string
		want      int
	}{
		{
			name:      "sums same item in same venue",
			venueSlug: "market-grizinkalna",
			itemID:    "item-a",
			want:      3,
		},
		{
			name:      "matches case insensitive and trimmed",
			venueSlug: "  MARKET-GRIZINKALNA ",
			itemID:    " ITEM-B ",
			want:      4,
		},
		{
			name:      "does not include other venue counts",
			venueSlug: "market-grizinkalna",
			itemID:    "item-c",
			want:      0,
		},
		{
			name:      "returns zero for empty venue",
			venueSlug: "",
			itemID:    "item-a",
			want:      0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := basketItemQuantityForVenue(baskets, tc.venueSlug, tc.itemID)
			if got != tc.want {
				t.Fatalf("unexpected quantity result: got %d want %d", got, tc.want)
			}
		})
	}
}

func TestIsRetryableRestoreModalClickError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "matches not attached wording",
			err:       errors.New(`playwright: Element is not attached to the DOM`),
			retryable: true,
		},
		{
			name:      "matches detached wording",
			err:       errors.New(`playwright: element is detached from DOM`),
			retryable: true,
		},
		{
			name:      "does not match other errors",
			err:       errors.New(`playwright: strict mode violation`),
			retryable: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryableRestoreModalClickError(tc.err)
			if got != tc.retryable {
				t.Fatalf("unexpected retryable result: got %v want %v", got, tc.retryable)
			}
		})
	}
}

func TestIsPlaywrightTimeoutError(t *testing.T) {
	if !isPlaywrightTimeoutError(errors.New("Timeout 30000ms exceeded")) {
		t.Fatalf("expected timeout detector to match timeout error")
	}
	if isPlaywrightTimeoutError(errors.New("some other failure")) {
		t.Fatalf("expected timeout detector to ignore non-timeout error")
	}
}
