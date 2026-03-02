# EAT CLI

EAT CLI is a plug-and-play skill that lets your AI agent browse menus, customize orders, and check out from major delivery services through a single command-line interface. Today, Wolt is fully implemented and Bolt is available as a stub integration.

## License

This project uses dual licensing:

1. AGPL-3.0-only (open source)
2. Commercial license (separate written agreement)

Commercial licensing is required for:
1. Closed-source redistribution.
2. Embedding in paid products.
3. SaaS/internal hosted use without AGPL source-sharing compliance.

See `LICENSE` and `COMMERCIAL_LICENSE.md`.

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
provider: "wolt"
success_url_pattern: "https://wolt.com/en/discovery"
success_selector: "[data-test-id='UserStatus.ProfileImage']"
user_data_dir: "./profile/wolt"
venue_base_url: "https://wolt.com/en/lva/riga"
headless: false
timeout_seconds: 600
```

`venue_base_url` controls geography-specific venue URL generation used by `basket add`, `basket remove`, and `checkout`. It is required and must include scheme + host (for example, `https://wolt.com/en/lva/riga`).
`provider` selects delivery service integration. Supported values:
- `wolt` (default): full implementation.
- `bolt`: stub provider (returns not-implemented errors for commands).

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

Use this one-time sign-in command to create or refresh a reusable session in `user_data_dir`.

Expected result:
- On success, the command prints `{"auth_status":"success"}` and later commands can run without signing in again.
- On failure, the command prints `{"auth_status":"failed","error":"..."}` and no usable session is saved.

Final command output is a JSON object with auth status:
- Success: `{"auth_status":"success"}`
- Failure: `{"auth_status":"failed","error":"..."}`
Auth launch respects `headless` from `config.yml`.

**Options:**
- `--erase-data`: Force deletion of existing session data before authenticating. Use this to start a fresh login session.

**Examples:**
```bash
go run main.go auth
go run main.go auth --erase-data
```

### `search` Command

This command searches for items and returns a JSON summary including the keyword, total count, and a `products` list. Each product includes `id`, `name`, `price`, `venue_id`, and `venue_slug`.

**Example:**
```bash
go run main.go search selga
go run main.go search peeled tomatoes
```

Example output shape:
```json
{
  "keyword": "selga",
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

Expected result:
- On success, it prints a normalized basket payload.
- If there is no valid authenticated session, it returns an error asking you to run `auth` first.

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

This command adds an item to a specific venue basket, or increments quantity if the item is already there.

Expected result:
- On success, it prints the updated basket JSON.
- If the session is not authenticated, it returns an error asking you to run `auth`.
- If the venue/item input is invalid, it returns an error.

Output uses the same `baskets` shape as `basket`.

**Example:**
```bash
go run main.go basket add wolt-market-grizinkalna 3135258a5f2ffa0c518ab4b8
```

### `basket remove` Command

This command removes a specific item from the selected venue basket and prints the resulting basket state as JSON.

Expected result:
- On success, the target item is removed from that venue basket and updated basket JSON is printed.
- If the item is not present in that venue basket, it returns an error.
- If the session is not authenticated, it returns an error asking you to run `auth`.

**Example:**
```bash
go run main.go basket remove wolt-market-grizinkalna 3135258a5f2ffa0c518ab4b8
```

### `checkout` Command

This command attempts to place an order for a selected venue.

Expected result:
- On success, the order is submitted.
- If checkout cannot be completed, the command returns an error payload with available provider error details.
- If the session is not authenticated, it returns an error asking you to run `auth` first.

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
