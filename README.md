# Go Playwright Authentication CLI

This is a Go application that uses Playwright to automate web tasks that require a persistent login session. It provides two commands: `auth` for interactive login and `search` for querying Wolt items with the saved session.

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
go run main.go <command> [options] [config.yml] [query]
```
-   `<command>` is `auth` or `search`.
-   `[options]` are command-specific flags.
-   `[config.yml]` is an optional path to your configuration file. It defaults to `config.yml` if not provided.
-   `[query]` (for `search` command) is the search term(s).

### `auth` Command

This command opens a browser window for you to perform a manual login. Your session data (cookies, local storage, etc.) will be saved to the `user_data_dir`.

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
