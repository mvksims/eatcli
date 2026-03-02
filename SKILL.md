---
name: eatcli
description: Use this skill when setting up, building, or running EAT CLI, a plug-and-play skill for AI agents to browse menus, customize orders, and check out across delivery services via CLI commands.
---

# EAT CLI Skill

EAT CLI is a plug-and-play skill that lets your AI agent browse menus, customize orders, and check out through one CLI workflow.

## Prerequisites

1. Go installed and available in `PATH` (project currently uses Go `1.25.6` in `go.mod`).
2. Playwright browsers installed for `playwright-go`.
3. A valid `config.yml` with required fields:
   - `provider`
   - `user_data_dir`
   - `venue_base_url` (required)
   - `timeout_seconds`
4. For Wolt ordering, an existing Wolt account with a saved payment card is required to place orders.

## Install (Cross-OS)

### macOS/Linux (bash/zsh)

```bash
git clone <repo-url> eatcli
cd eatcli
```

### Windows (PowerShell)

```powershell
git clone <repo-url> eatcli
Set-Location .\eatcli
```

## Build

### macOS/Linux

```bash
go mod tidy
go run github.com/playwright-community/playwright-go/cmd/playwright install
mkdir -p bin
go build -o bin/eatcli ./main.go
```

### Windows (PowerShell)

```powershell
go mod tidy
go run github.com/playwright-community/playwright-go/cmd/playwright install
New-Item -ItemType Directory -Force -Path .\bin | Out-Null
go build -o .\bin\eatcli.exe .\main.go
```

## Usage Notes

1. Run `auth` first to create or refresh session state:
```bash
go run main.go auth
```
2. `search`, `basket`, and `checkout` rely on authenticated session data in `user_data_dir`.
3. Current provider support is Wolt only for real ordering flows; Bolt is a stub integration.

## Ordering Rules

1. Prefer ordering items from the same `venue_slug` to reduce delivery and commission costs.
2. Before running `checkout`, always warn the human user about current basket total and ask for confirmation.
3. Only packed/simple items are currently supported in ordering flows.
4. Items sold by weight are not supported at the moment.

## Usage: Search

```bash
go run main.go search potato
go run main.go search peeled tomatoes
```

## Usage: Basket

```bash
# View basket
go run main.go basket

# Add item to basket
go run main.go basket add <venue_slug> <item_id>

# Remove item from basket
go run main.go basket remove <venue_slug> <item_id>
```

## Usage: Checkout

```bash
go run main.go checkout <venue_slug>
```
