# Agent Notes for EAT CLI

This document contains notes and context from the Gemini CLI agent regarding the development and functionality of this application. It should be updated whenever the application's logic or architecture changes significantly.

## Project Summary

This application implements a Go-based Command Line Interface (CLI) leveraging Playwright for browser automation. Its primary purpose is to manage persistent login sessions for web applications, allowing for command-based flows:
1.  **`auth` command:** An interactive process where a user manually logs into a web service (e.g., Wolt) in a browser window. The browser session is then persisted to a specified `user_data_dir`.
2.  **`search` command:** A non-interactive process that reuses the persisted session to search for items on Wolt.
3.  **`basket` command:** Returns the current basket payload as JSON for the active session.
4.  **`basket add` / `basket remove` commands:** Non-interactive flows that open a specific Wolt item page using `venue_slug` and `item_id`, wait for load, click either add/decrement product controls, capture the baskets API response, and print JSON.
5.  **`checkout` command:** A non-interactive flow that opens a venue checkout page using `venue_slug`, waits for full load, and clicks the Send Order button.

## Key Technologies and Architecture

*   **Language:** Go
*   **Web Automation:** `playwright-go` library (`github.com/playwright-community/playwright-go`)
*   **Configuration:** YAML files (`gopkg.in/yaml.v3`)
*   **Authentication Strategy:** Uses Playwright's `LaunchPersistentContext` to save and reuse browser profiles (including cookies, local storage, etc.) for session persistence. Login detection relies on `success_url_pattern` and `success_selector` defined in the config.
*   **Browser Configuration:** Playwright is configured to use the Firefox browser engine with a Firefox user agent. Automation indicators (like `navigator.webdriver`) are hidden using an injected init script.

## Development History and Debugging Insights

During initial development, several challenges were encountered, primarily revolving around Playwright's `WaitFor*` functions and the "target closed" error:

*   **Compilation Errors:**
    *   Initial `page.WaitForURL` assignment mismatch (fixed `_, err = ...` to `err = ...`).
    *   `playwright.PageLoadStateNetworkIdle` undefined (fixed to `playwright.LoadStateNetworkidle`).
    *   `playwright.LoadStateNetworkIdle` case sensitivity (fixed to `playwright.LoadStateNetworkidle`).
    *   `page.WaitForLoadState` argument type mismatch (fixed by passing `playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle}`).
*   **"Target Closed" Error during `auth`:**
    *   This was the most persistent issue. It occurred when `WaitForLoadState`, `WaitForURL`, or `WaitForSelector` reported that the target page/context was closed, even though `defer` statements were correctly placed.
    *   **Attempted Fixes:**
        *   Ensured `UserDataDir` was an absolute path.
        *   Added `page.WaitForLoadState("networkidle")` after `page.Goto`.
        *   Added `page.WaitForURL(cfg.StartURL)` after `page.WaitForLoadState`.
        *   Temporarily removed `defer` statements for diagnostics (reverted).
        *   Attempted to isolate `WaitForURL` vs. `WaitForSelector` by modifying `config.yml`.
        *   Changed `WaitForLoadState` to `playwright.LoadStateDomcontentloaded` for initial page stability.
    *   **Current Status:** The "target closed" error still occurs during the `auth` command's `page.WaitForLoadState` or `page.WaitForURL` calls for Wolt's login page, suggesting a complex interaction or redirection within the target website itself that invalidates the Playwright `page` object. The issue arises *before* user interaction, at the initial page load. Further debugging may require direct browser observation or alternative waiting strategies (e.g., polling).

-   **Optional Config Argument:** The CLI was updated to make the `config.yml` argument optional. It now defaults to `config.yml` if no path is provided.
-   **Search Output Expansion:** The `search` command now returns a `products` array with per-product metadata (`id`, `name`, `price`, `venue_id`, `venue_slug`) in addition to the keyword and count.
-   **Search Payload Parsing:** The `search` parser now supports Wolt item payloads where fields are nested under `items[].menu_item` and/or `items[].link.menu_item_details`, which fixed missing `id`, `price`, and `venue_slug` in results.
-   **Basket View Command:** Added `basket` (without subcommands) to return the current basket JSON.
-   **Basket Add Command:** `basket add <venue_slug> <item_id>` now waits for page load, clicks `[data-test-id="product-modal.total-price"]`, captures a successful `GET` response for `https://consumer-api.wolt.com/order-xp/web/v1/pages/baskets`, and prints the response JSON.
-   **Basket Remove Command:** Added `basket remove <venue_slug> <item_id>` with the same flow as `basket add`, but it clicks `[data-test-id="product-modal.quantity.decrement"]` before collecting the baskets API response JSON.
-   **Basket Output Shape:** Basket commands now normalize output into a `baskets` array where each basket includes `id`, `total`, `venue_slug`, and `items` (`id`, `count`, `total`, `image_url`, `name`, `is_available`, `price`).
-   **Basket Restore Modal Handling:** Basket item actions now wait up to 30 seconds for `[data-test-id="restore-order-modal.confirm"]` after initial page load and click it when present before attempting add/remove.
-   **Checkout Command:** Added `checkout <venue_slug>` to open `https://wolt.com/en/lva/riga/venue/<venue_slug>/checkout` and click `[data-test-id="SendOrderButton"]` after full page load.
-   **Checkout Error Modal Output:** After clicking `SendOrderButton`, `checkout` now waits up to 10 seconds for `GenericCheckoutErrorModal` and includes its inner text in output when present.

## Usage Notes for Agent

*   When running the `auth` command, remember it's interactive. The user *must* manually log in within the opened browser window.
*   The `user_data_dir` is crucial for session persistence. Ensure it's not committed to version control (`.gitignore` takes care of this).
*   If "target closed" errors reappear, re-evaluate the target website's navigation behavior during login, especially for redirects or frame changes, and adjust `WaitFor*` calls or implement polling.
*   After *any* code change, the agent is required to run the comprehensive test suite (`go test -v`) to ensure functionality and prevent regressions.
*   **Commit Message Format:** When committing changes, use the format 'type: change description' with a prefix reflecting the type of change (e.g., 'feat: add new feature', 'fix: resolve bug', 'docs: update documentation').

---
**NOTE:** This file should be updated whenever the application's logic, dependencies, or architectural decisions are changed to maintain an accurate history and guide future maintenance.

**IMPORTANT:** If the application's name, commands, or arguments are changed, you *must* also update the `README.md` file to reflect these changes.
