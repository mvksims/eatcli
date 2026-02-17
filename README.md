# Go Playwright Authentication CLI

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
headless: false
timeout_seconds: 600
```

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

**Options:**
- `--erase-data`: Force deletion of existing session data before authenticating. Use this to start a fresh login session.

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
- If the item is already in that venue basket, it opens checkout and increments from the cart item modal.
- If the item is not in cart, it opens the item detail page, waits for and confirms `restore-order-modal.confirm`, then clicks `product-modal.submit`, waits for refreshed baskets API response, and prints normalized basket JSON.

Output uses the same `baskets` shape as `basket`.

**Example:**
```bash
go run main.go basket add wolt-market-grizinkalna 3135258a5f2ffa0c518ab4b8
```

### `basket remove` Command

This command removes a specific item from the selected venue basket and prints the resulting basket state as JSON.

Flow:
- It first checks basket state and confirms the requested item ID exists in the same venue basket.
- It reads the current item quantity from basket data.
- It opens checkout, opens the cart item modal, clicks `product-modal.quantity.decrement` the same number of times as the current quantity, then clicks `product-modal.submit`.
- It waits for updated baskets API response and prints normalized `baskets` output.

**Example:**
```bash
go run main.go basket remove wolt-market-grizinkalna 3135258a5f2ffa0c518ab4b8
```

### `checkout` Command

This command attempts to place the order for a venue by triggering the send-order action.

After attempting checkout, it waits up to 10 seconds for `GenericCheckoutErrorModal`. If it appears, the command returns the modal message in `generic_checkout_error_modal`.

**Example:**
```bash
go run main.go checkout wolt-market-grizinkalna
```
