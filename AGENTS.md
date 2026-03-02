# Agent Notes for EAT CLI

This document contains notes and context from the agent regarding the development and functionality of this application. It should be updated whenever the application's logic or architecture changes significantly.

## Project Summary

EAT CLI is a plug-and-play skill that lets an AI agent browse menus, customize orders, and check out from major delivery services through one CLI interface. The current implementation is provider-based, with Wolt as the full integration and Bolt as a stub.

The application implements a Go-based Command Line Interface (CLI) leveraging Playwright for browser automation, with the following command-based flows:
1.  **`auth` command:** An interactive process where a user manually logs into a web service (e.g., Wolt) in a browser window. Before reporting success, it verifies `UserStatusDropdown`; only then is the browser session considered persisted to `user_data_dir`.
2.  **`search` command:** A non-interactive process that reuses the persisted session to search for items on Wolt.
3.  **`basket` command:** Returns the current basket payload as JSON for the active session, and verifies login presence using `UserStatusDropdown` after the initial page load.
4.  **`basket add` command:** A non-interactive flow that first checks baskets API data for `item_id` within the requested `venue_slug`. If present, it opens checkout and increments quantity through the cart item modal; if absent, it opens the item detail page, confirms restore-order modal, clicks submit, and captures the refreshed baskets API response to print normalized JSON.
5.  **`basket remove` command:** A non-interactive flow that first checks baskets API data for `item_id` within the requested `venue_slug`, remembers its quantity, opens checkout, verifies checkout readiness via `SendOrderButton`, and when needed recovers by optionally confirming restore-order modal, clicking `cart-view-button` when shown, then clicking cart next-step before decrementing exactly that many times in the cart item modal, submitting, and printing normalized basket JSON.
6.  **`checkout` command:** A non-interactive flow that opens a venue checkout page using `venue_slug`, waits for full load, verifies login presence using `UserStatusDropdown`, and clicks the Send Order button.

## Key Technologies and Architecture

*   **Language:** Go
*   **Web Automation:** `playwright-go` library (`github.com/playwright-community/playwright-go`)
*   **Configuration:** YAML files (`gopkg.in/yaml.v3`)
*   **Authentication Strategy:** Uses Playwright's `LaunchPersistentContext` to save and reuse browser profiles (including cookies, local storage, etc.) for session persistence. Login detection relies on `success_url_pattern` and `success_selector` defined in the config.
*   **Browser Configuration:** Playwright is configured to use the Firefox browser engine with a Firefox user agent. Automation indicators (like `navigator.webdriver`) are hidden using an injected init script.

## Usage Notes for Agent
*   The `user_data_dir` is crucial for session persistence. Ensure it's not committed to version control (`.gitignore` takes care of this).
*   If "target closed" errors reappear, re-evaluate the target website's navigation behavior during login, especially for redirects or frame changes, and adjust `WaitFor*` calls or implement polling.
*   After *any* code change, the agent is required to run the comprehensive test suite (`go test -v`) to ensure functionality and prevent regressions.
*   **Commit Message Format:** When committing changes, use the format 'type: change description' with a prefix reflecting the type of change (e.g., 'feat: add new feature', 'fix: resolve bug', 'docs: update documentation').

---
**NOTE:** This file should be updated whenever the application's logic, dependencies, or architectural decisions are changed to maintain an accurate history and guide future maintenance.

**IMPORTANT:** If the application's name, commands, or arguments are changed, you *must* also update the `README.md` file to reflect these changes.
