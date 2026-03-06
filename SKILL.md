---
name: eatcli
description: >-
  Run and troubleshoot the eatcli delivery automation Go CLI for auth, search,
  basket, and checkout flows. Use when asked to authenticate session, add/remove
  basket items, run checkout attempts, debug Playwright launch issues, or
  operate eatcli with config.yml in Docker/headless environments.
---

# eatcli

Use this skill to run `eatcli` safely in containerized/headless environments.

## Install/use from public repository

- Repository: `https://github.com/mvksims/eatcli`
- Clone the repository into your agent skill storage and keep the folder name as `eatcli`.
- If skills are not auto-discovered, reload/restart the agent runtime.

## Run commands

From the `eatcli` skill directory:

Help:

```bash
go run . --help
```

Auth (without data wipe):

```bash
go run . auth config.yml
```

Search:

```bash
go run . search "<query>" config.yml
```

Basket:

```bash
# View basket
go run . basket config.yml

# Add item
go run . basket add <venue_slug> <item_id> config.yml

# Remove item
go run . basket remove <venue_slug> <item_id> config.yml
```

Checkout:

```bash
go run . checkout <venue_slug> config.yml
```

Important: do not use `auth --erase-data` unless user explicitly requests data reset.

## Config expectations

Required fields in `config.yml`:
- `success_url_pattern`
- `success_selector`
- `user_data_dir`
- `venue_base_url`
- `headless`
- `timeout_seconds`

## Headless requirement

If running without X server, set:

```yaml
headless: true
```

If headed mode is required, use an X server (`xvfb-run`) in compatible environments.

## Safety and ordering rules

- Never run checkout silently; confirm with user before `checkout`.
- Prefer one venue per order unless user requests split baskets.
- If runtime fails after auth, re-check session validity and retry once.

## Smoke test

1. `go run . --help`
2. `go test ./...`
3. `go run . search "selga cepumi" config.yml`
4. `go run . basket config.yml`
