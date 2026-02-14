# Go Playwright Authentication CLI

This is a Go application that uses Playwright to automate web tasks that require a persistent login session. It provides two commands: `auth` for interactive login, and `run` for executing automation.

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

The application is configured via a `config.yml` file, which should be in the same directory. The file specifies the login URL, success conditions, and other settings.

Example `config.yml`:
```yaml
start_url: "https://wolt.com/en/login"
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
-   `<command>` is `auth`, `run`, or `search`.
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

### `run` Command

This command uses the saved session to run the automation logic. If the session has expired, it will instruct you to run the `auth` command again.

**Example:**
```bash
go run main.go run
```

### `search` Command

This command searches for items on Wolt and returns a JSON summary including the keyword and the count of items found.

**Example:**
```bash
go run main.go search potato
go run main.go search peeled tomatoes
```
