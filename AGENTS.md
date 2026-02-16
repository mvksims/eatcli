# Agent Notes for EAT CLI

This document contains notes and context from the Gemini CLI agent regarding the development and functionality of this application. It should be updated whenever the application's logic or architecture changes significantly.

## Project Summary

This application implements a Go-based Command Line Interface (CLI) leveraging Playwright for browser automation. Its primary purpose is to manage persistent login sessions for web applications, allowing for a two-stage workflow:
1.  **`auth` command:** An interactive process where a user manually logs into a web service (e.g., Wolt) in a browser window. The browser session is then persisted to a specified `user_data_dir`.
2.  **`search` command:** A non-interactive process that reuses the persisted session to search for items on Wolt.

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

## Usage Notes for Agent

*   When running the `auth` command, remember it's interactive. The user *must* manually log in within the opened browser window.
*   The `user_data_dir` is crucial for session persistence. Ensure it's not committed to version control (`.gitignore` takes care of this).
*   If "target closed" errors reappear, re-evaluate the target website's navigation behavior during login, especially for redirects or frame changes, and adjust `WaitFor*` calls or implement polling.
*   After *any* code change, the agent is required to run the comprehensive test suite (`go test -v`) to ensure functionality and prevent regressions.
*   **Commit Message Format:** When committing changes, use the format 'type: change description' with a prefix reflecting the type of change (e.g., 'feat: add new feature', 'fix: resolve bug', 'docs: update documentation').

---
**NOTE:** This file should be updated whenever the application's logic, dependencies, or architectural decisions are changed to maintain an accurate history and guide future maintenance.

**IMPORTANT:** If the application's name, commands, or arguments are changed, you *must* also update the `README.md` file to reflect these changes.
