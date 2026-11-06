# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

BEpusdt is a self-hosted crypto payment gateway (Go backend + Vue admin UI). It scans blockchains for incoming stablecoin/native transfers (USDT/USDC/TRX/BNB/ETH/TON on Tron, Ethereum, BSC, Polygon, Arbitrum, Base, X-Layer, Plasma, Solana, Aptos, Ton), matches them to pending orders, and calls back the merchant. It is API-compatible with Epusdt and with the 易支付 (Epay) `submit.php` protocol. Docs are in Chinese; code comments are mostly Chinese too. Module path: `github.com/v03413/bepusdt`.

PR convention (from `main/main.go`): one PR does one thing, and the description states what changed, why, where review should focus, and how it was tested.

## Commands

Backend (Go 1.26, `CGO_ENABLED=0`, pure-Go SQLite via `glebarez/sqlite`):

```bash
go build -o bepusdt ./main          # build
go run ./main start                 # run; flags below can also come from env or a .env file
go test ./app/...                   # all tests
go test ./app/task -run TestName    # single test
go test ./app/task/notify           # one package
```

Subcommands: `start`, `version`, `reset` (resets admin login). `start` flags/env: `--listen`/`LISTEN` (default `:8080`), `--sqlite`/`SQLITE` (default `/var/lib/bepusdt/sqlite.db`), `--postgres`/`POSTGRESQL_DSN` (if set, used instead of SQLite), `--log`/`LOG` (default `/var/log/bepusdt/`). `main` loads `.env` via godotenv; see `.env.example`. For local runs, point `SQLITE` and `LOG` at writable paths.

Frontend (`web/`, Vue 3 + Vite, pnpm 10 only, enforced by a preinstall hook; admin UI is the SnowAdmin template):

```bash
cd web && pnpm install --frozen-lockfile --shamefully-hoist
pnpm dev               # dev server; proxies API to VITE_APP_BASE_URL (http://127.0.0.1:8080 in .env.development)
pnpm build:prod        # vue-tsc + vite build -> web/dist
pnpm lint:eslint       # eslint --fix
pnpm lint:prettier
```

Production build order matters: the built frontend (`web/dist/*`) must be copied to `static/secure/` **before** `go build`, because `static/embep.go` embeds `secure/*`, `payment/*` and `checkout`. The `dockerfile` and `.github/workflows/release.yml` (GoReleaser) both do this. Version is injected with `-ldflags "-X github.com/v03413/bepusdt/app.Version=..."`. `static/secure/` is otherwise empty (only a `.gitignore`), so `go build` works without the frontend, but the admin page will be missing.

CI (`pr-check.yml`) only does a Docker build; there is no CI test or lint step.

Test notes: on Windows, `app/task/notify` tests can fail at `TempDir` cleanup because the SQLite file is still locked, and `TestSolanaSlotParseSkipsFailedTransactions` failed in one run on Windows (not investigated). Check whether a failure is platform-related before assuming your change caused it.

## Architecture

Entry: `main/main.go` -> `app/cmd/start.go`. Startup order: `model.Init` (DB + migrations + default config) -> `log.Init` -> `task.Init()` -> `task.Start(ctx)` (background workers) -> Gin HTTP server (`router.Handler()`).

### Task scheduler (`app/task`)

`task.go` holds a global registry. Each file registers workers with `Register(Task{Callback, Duration})`, usually in `init()`. `Duration == 0` means run the callback once and let it loop itself (used for long-running consumers); otherwise it runs immediately and then on a ticker. EVM chains are registered from `Init()` through `bscInit()`, `ethInit()`, etc., because they need runtime config. Tron, Solana, Aptos and Ton register in `init()`.

Every chain follows the same four-worker pipeline:
1. `sync*Forward`: poll the RPC for the chain head and enqueue block ranges onto an unbounded channel (`chanx`). `syncBreak` stops enqueueing when the queue exceeds `blockQueueLimit` (100).
2. `*Dispatch`: consume the queue, fetch and parse blocks (worker pool via `ants`), and emit normalized `transfer` structs onto the global `transferQueue`.
3. `tradeConfirmHandle`: confirm orders after enough block confirmations.
4. `lookback*`: re-scan history for orders that may have been missed (once per order, tracked by `lookbackDone`).

All EVM chains share the `evm` struct in `evm.go`. A new EVM chain is a small file such as `bsc.go` that sets network, confirm offset, native-coin parsing, and the four registrations. Tron (`tron.go`, gRPC), Solana, Aptos and Ton have their own implementations.

`transfer.go` is the convergence point. `orderTransferHandle` batches transfers every second, filters by `model.IsAmountValid`, publishes to MQTT, matches against receivable orders keyed by `recvAddress + tradeType`, and calls `order.MarkConfirming`. Unmatched transfers go to `notOrderTransferHandle`. Matching is done by `orderTransferMatch` and depends on the `payment_match_mode` config (`classic` exact, `has_prefix` truncate to atomic precision, `round_off`). Tests are in `amount_match_test.go`.

Other workers: `rate.go` (fiat/crypto exchange rates), `conf.go` (refreshes the in-memory config cache every 3s), `notify.go` + `task/notify` (merchant callbacks with retry), `mqtt.go`.

### Model (`app/model`)

- `registry.go` is the single source of truth for every supported `TradeType`: network, contract, decimals, amount range, explorer URL, RPC endpoint config key, address case sensitivity. Adding a token or chain starts here (plus constants in `app/conf/const.go`).
- Settings live in the DB `Conf` table, read through `model.GetC(key)` from an in-memory cache refreshed by `RefreshC()`. Defaults are in `model/conf.go` and backfilled by `FillDefaultConf()`. RPC endpoints are configured in this table, not in env.
- Schema: `AutoMigrate` runs GORM auto-migration for `Wallet`, `Order`, `NotifyRecord`, `Conf`, `Rate`, then runs ordered gormigrate migrations from `app/model/migration/` (table `bep_migration`). Add data/schema migrations there, named `<yyyymmddhhmm>_<desc>.go`, and append them to the `migrations` slice.
- SQLite is the default (WAL, max 5 open connections); PostgreSQL is used when a DSN is given. Code must work on both.

### HTTP (`app/router`, `app/handler`)

- `router.go` wraps route registration with `PostRegister`/`GetRegister`, which record each route in `authRoute` (needs the admin token) and `secureRoute` (needs the session set by visiting the secret "secure entry" path). A route registered with plain `engine.GET/POST` bypasses both checks.
- Merchant-facing APIs: `/api/v1/order/*` and `/api/v1/pay/*` (Epusdt-compatible, signed with the API token), `/submit.php` (Epay order submit) and `/api.php?act=order` (Epay order query; authenticates with `key` = API token in the query string, so the Gin access logger is configured with `SkipQueryString`). The checkout page is `/pay/checkout/:trade_id`.
- `handler/admin` + `handler/auth` serve the Vue admin UI; `handler/base` holds shared helpers.
- Checkout templates: `static/checkout/{official,langge}` are embedded. If a `static/checkout` directory exists next to the working directory at runtime, it overrides the embedded set (`checkoutSource()` in `router/static.go`). Each template needs a `checkout.json`.

### Other packages

`app/notifier` (Telegram/WeChat/none order notifications), `app/mqtt` (publishes every matched transfer, subscribes for control messages), `app/core` (client for the external `api.bepusdt.online` service), `app/log` (logrus + lumberjack; `log.Task` is the logger for workers), `app/utils`.

### Test setup

`app/task/testenv_test.go` provides `initTaskTestEnv`, which creates a temp SQLite DB and log dir and sets config overrides. Solana tests replay mainnet transaction fixtures from `app/task/testdata/solana/`. Tests use the real `model.Db` global, so they cannot run in parallel.

## Docs

`docs/` holds user-facing docs (install, API integration for each shop platform, notify callback spec, payment match modes). Update `docs/trade-type.md` when adding a supported network or token, and `docs/api/` when changing the merchant API.
