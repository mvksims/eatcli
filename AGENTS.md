# Agent Notes for EAT CLI

This document contains notes and context from the Gemini CLI agent regarding the development and functionality of this application. It should be updated whenever the application's logic or architecture changes significantly.

## Project Summary

This application implements a Go-based Command Line Interface (CLI) leveraging Playwright for browser automation. Its primary purpose is to manage persistent login sessions for web applications, allowing for command-based flows:
1.  **`auth` command:** An interactive process where a user manually logs into a web service (e.g., Wolt) in a browser window. The browser session is then persisted to a specified `user_data_dir`.
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
-   **Basket Add Cart-Aware Branch:** `basket add` now first checks basket JSON for `item_id` within the requested venue; when present, it opens checkout, locates `div[data-test-id="CartItem"][data-value="<item_id>"]`, opens the item modal from cart, increments quantity, and reads basket JSON.
-   **Basket Add Direct Item Flow Rewrite:** When the item is not yet in basket, `basket add` now runs a dedicated flow: open item detail URL, optionally confirm `[data-test-id="restore-order-modal.confirm"]` when shown, wait for/click `[data-test-id="product-modal.submit"]`, then wait for updated baskets API response and print normalized basket JSON.
-   **Basket Remove Command:** `basket remove <venue_slug> <item_id>` now checks basket JSON for item presence and quantity in the requested venue, opens checkout, verifies readiness by checking `[data-test-id="SendOrderButton"]`, and if it is not visible optionally confirms `[data-test-id="restore-order-modal.confirm"]`, clicks `[data-test-id="cart-view-button"]` when shown, then clicks `[data-test-id="CartViewNextStepButton"]` before opening the item modal from cart, clicking `[data-test-id="product-modal.quantity.decrement"]` exactly quantity times, clicking `[data-test-id="product-modal.submit"]`, and printing updated basket JSON.
-   **Basket Output Shape:** Basket commands now normalize output into a `baskets` array where each basket includes `id`, `total`, `venue_slug`, and `items` (`id`, `count`, `total`, `image_url`, `name`, `is_available`, `price`).
-   **Basket Restore Modal Handling:** Basket add direct item flow now treats `[data-test-id="restore-order-modal.confirm"]` as optional and continues when it does not appear within timeout. Checkout cart-aware increment path does not run this modal step.
-   **Checkout Command:** Added `checkout <venue_slug>` to open `https://wolt.com/en/lva/riga/venue/<venue_slug>/checkout` and click `[data-test-id="SendOrderButton"]` after full page load.
-   **Checkout Error Modal Output:** After clicking `SendOrderButton`, `checkout` now waits up to 10 seconds for `GenericCheckoutErrorModal` and includes its inner text in output when present.
-   **Basket/Checkout Login Guard:** Basket and checkout flows now validate `[data-test-id="UserStatusDropdown"]` after initial page load; when absent, commands fail fast and instruct running `auth` first.
-   **User Data Dir Safety Validation:** Config loading now rejects empty/root `user_data_dir`, and `auth --erase-data` refuses destructive targets such as filesystem root, home directory, and current working directory.
-   **Stale Automation Path Cleanup:** Removed unused internal automation-only code paths and related tests (`runAutomation`, `waitAuthorized*`, and `runBasketItemAction`) to keep command behavior aligned with supported CLI surface.
-   **Shared Browser Bootstrap Helper:** Added a common Playwright session launcher that centralizes persistent context startup, anti-detection init script, viewport setup, and request header routing across auth/search/basket/checkout flows.
-   **Configurable Geography Base:** Added optional `venue_base_url` configuration used by venue-scoped URL builders for `basket add`, `basket remove`, and `checkout` flows. Defaults to `https://wolt.com/en/lva/riga` when not configured.

## Usage Notes for Agent

*   When running the `auth` command, remember it's interactive. The user *must* manually log in within the opened browser window.
*   The `user_data_dir` is crucial for session persistence. Ensure it's not committed to version control (`.gitignore` takes care of this).
*   If "target closed" errors reappear, re-evaluate the target website's navigation behavior during login, especially for redirects or frame changes, and adjust `WaitFor*` calls or implement polling.
*   After *any* code change, the agent is required to run the comprehensive test suite (`go test -v`) to ensure functionality and prevent regressions.
*   **Commit Message Format:** When committing changes, use the format 'type: change description' with a prefix reflecting the type of change (e.g., 'feat: add new feature', 'fix: resolve bug', 'docs: update documentation').

---
**NOTE:** This file should be updated whenever the application's logic, dependencies, or architectural decisions are changed to maintain an accurate history and guide future maintenance.

**IMPORTANT:** If the application's name, commands, or arguments are changed, you *must* also update the `README.md` file to reflect these changes.
