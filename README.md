# EAT CLI

This is a Go application that uses Playwright to automate shopping workflows with a persistent login session. It provides `auth` to sign in once, `search` to find products, `basket` to read current basket state, `basket add` to increase item quantity, `basket remove` to remove items from basket, and `checkout` to attempt order placement and surface checkout errors.

## Prerequisites

- Go (v1.18 or later) installed and configured in your system's PATH.

## Setup

1.  **Install Go dependencies:**
    ```bash
    go mod tidy
    ```

2.  **Install Playwright browser binaries:**
    ```bash
    go run github.com/playwright-community/playwright-go/cmd/playwright install
    ```

## Configuration

The application is configured via a `config.yml` file, which should be in the same directory. The file specifies success conditions, session directory, and runtime settings.

Example `config.yml`:
```yaml
success_url_pattern: "https://wolt.com/en/discovery"
success_selector: "[data-test-id='UserStatus.ProfileImage']"
user_data_dir: "./profile/wolt"
venue_base_url: "https://wolt.com/en/lva/riga"
headless: false
timeout_seconds: 600
```

`venue_base_url` controls geography-specific venue URL generation used by `basket add`, `basket remove`, and `checkout`. If omitted, it defaults to `https://wolt.com/en/lva/riga`.

## Usage

The application is run with the following structure:
```bash
go run main.go <command> [options] [config.yml] [args...]
```
-   `<command>` is `auth`, `search`, `basket`, or `checkout`.
-   `[options]` are command-specific flags.
-   `[config.yml]` is an optional path to your configuration file. It defaults to `config.yml` if not provided.
-   `[query]` (for `search` command) is the search term(s).
-   `basket` with no additional arguments returns current basket JSON.
-   For `basket add` and `basket remove`, arguments are `<venue_slug> <item_id>`.
-   For `checkout`, argument is `<venue_slug>`.

### `auth` Command

This command is your one-time sign-in step. It stores your session in `user_data_dir` so shopping commands can reuse it without requiring login each time.
Before reporting success, it verifies `[data-test-id="UserStatusDropdown"]`. If that marker is not visible, `auth` returns a failure instead of persisting a successful result.
Final command output is a simple JSON object with auth status:
- Success: `{"auth_status":"success"}`
- Failure: `{"auth_status":"failed","error":"..."}`
Auth launch respects `headless` from `config.yml` while keeping the existing browser automation steps.

**Options:**
- `--erase-data`: Force deletion of existing session data before authenticating. Use this to start a fresh login session.

Input requirement:
- The sign-in URL entered in the prompt must be on `wolt.com` (including subdomains such as `www.wolt.com`).
- If not, auth fails with JSON status: `{"auth_status":"failed","error":"auth URL is incorrect: domain must be wolt.com"}`.

**Examples:**
```bash
go run main.go auth
go run main.go auth --erase-data
```

### `search` Command

This command searches for items on Wolt and returns a JSON summary including the keyword, total count, and a `products` list. Each product includes `id`, `name`, `price`, `venue_id`, and `venue_slug`.

**Example:**
```bash
go run main.go search potato
go run main.go search peeled tomatoes
```

Example output shape:
```json
{
  "keyword": "potato",
  "count": 2,
  "products": [
    {
      "id": "a22bc220dd44c8f8daa8ef96",
      "name": "Selga šokolādes glazūrā 190g cepumi",
      "price": 289,
      "venue_id": "62430901d7678f5b344972e4",
      "venue_slug": "wolt-market-grizinkalna"
    }
  ]
}
```

### `basket` Command

This command returns your current basket as JSON.
After the initial page load, it verifies `[data-test-id="UserStatusDropdown"]`; if missing, it treats the session as logged out and asks you to run `auth` first.

Output contains a `baskets` array. Each basket includes:
- `id`
- `total`
- `venue_slug`
- `items` (each item includes `id`, `count`, `total`, `image_url`, `name`, `is_available`, `price`)

**Example:**
```bash
go run main.go basket
```

### `basket add` Command

This command increases quantity for a specific item in your basket and prints the resulting basket state as JSON.

Flow:
- It first checks basket state and looks for the requested item ID inside the same venue basket.
- It verifies login status after initial page loads using `[data-test-id="UserStatusDropdown"]`; if missing, it stops and asks you to run `auth` first.
- If the item is already in that venue basket, it opens checkout and increments from the cart item modal.
- If the item is not in cart, it opens the item detail page, optionally confirms `restore-order-modal.confirm` when shown, then clicks `product-modal.submit`, waits for refreshed baskets API response, and prints normalized basket JSON.

Output uses the same `baskets` shape as `basket`.

**Example:**
```bash
go run main.go basket add wolt-market-grizinkalna 3135258a5f2ffa0c518ab4b8
```

### `basket remove` Command

This command removes a specific item from the selected venue basket and prints the resulting basket state as JSON.

Flow:
- It first checks basket state and confirms the requested item ID exists in the same venue basket.
- It verifies login status after initial page loads using `[data-test-id="UserStatusDropdown"]`; if missing, it stops and asks you to run `auth` first.
- It reads the current item quantity from basket data.
- After checkout loads, it checks if `SendOrderButton` is visible; if not, it optionally confirms `restore-order-modal.confirm`, clicks `cart-view-button` when shown, then clicks `CartViewNextStepButton`, and waits for checkout readiness.
- It opens checkout, opens the cart item modal, clicks `product-modal.quantity.decrement` the same number of times as the current quantity, then clicks `product-modal.submit`.
- It waits for updated baskets API response and prints normalized `baskets` output.

**Example:**
```bash
go run main.go basket remove wolt-market-grizinkalna 3135258a5f2ffa0c518ab4b8
```

### `checkout` Command

This command attempts to place the order for a venue by triggering the send-order action.
After the initial page load, it verifies `[data-test-id="UserStatusDropdown"]`; if missing, it treats the session as logged out and asks you to run `auth` first.

After attempting checkout, it waits up to 10 seconds for `GenericCheckoutErrorModal`. If it appears, the command returns the modal message in `generic_checkout_error_modal`.

**Example:**
```bash
go run main.go checkout wolt-market-grizinkalna
```

## Integration Harness

An end-to-end harness is available as an integration test with build tag `integration`.

Scenario covered:
1. Search for product query #1.
2. Search for product query #2.
3. Pick two products from the same venue.
4. Add product #1 to basket.
5. Add product #2 to basket.
6. Remove product #1 from basket.
7. Verify product #1 is gone and product #2 remains in that venue basket.

Run it with:
```bash
EATCLI_E2E=1 \
EATCLI_E2E_CONFIG=config.yml \
EATCLI_E2E_QUERY_ONE=milk \
EATCLI_E2E_QUERY_TWO=bread \
go test -tags integration -run TestIntegrationHarness_SearchAddAddRemoveSameRetailer -v
```

Environment variables:
- `EATCLI_E2E` (required): set to `1` to enable the harness test.
- `EATCLI_E2E_CONFIG` (optional): config path, defaults to `config.yml`.
- `EATCLI_E2E_QUERY_ONE` (optional): first search query, defaults to `milk`.
- `EATCLI_E2E_QUERY_TWO` (optional): second search query, defaults to `bread`.
