# telegram-bot-pooling-hard

Level 2–3 Telegram bot template: service booking (клиника / салон) плюс задел под «продвинутый» бот (баланс, рефералы, i18n, Redis, аналитика). Целевой профиль Level 3 описан в [RFC.md](RFC.md).

This repository is designed as a more advanced and sellable version of the first bot level, while keeping a similar project structure for easier maintenance and future feature development.

## Product Specification (Level 2)

### Goal

Build a medium-complexity Telegram bot for service appointments.

### Core features

- Step-by-step conversational flows (state machine / wizard).
- Persistent data storage in PostgreSQL or MySQL.
- Basic in-bot owner admin panel:
  - broadcast management;
  - simple statistics viewing.

### Integrations

- Bitrix24 CRM.
- Email notifications.
- HTTP webhooks.

### Tech requirements

- Go service + database.
- Long polling mode.
- Layered project structure: `transport -> service -> repository`.

## Current Repository Status

The project includes a Go bot scaffold, tests, Docker packaging, and CI/CD (`.github/workflows/ci.yml`, `release.yml`, `deploy.yml`).  
**Booking:** MVP wizard с записью к врачу, отменой, документами, админ-инструментами по слотам.  
**Level 3 (RFC):** профиль пользователя (`user_profiles`), списание баланса за запись, реферальные бонусы, RU/EN, события аналитики, опциональный Redis для кеша списка специализаций. Миграция `006_level3_profiles_analytics.sql`.

### Implemented MVP Wizard Flow

- `/book` starts a finite-state booking flow.
- User selects service by number.
- User selects available slot by number.
- User confirms with `YES` (or cancels with `NO` / `/cancel`).
- Booking is persisted and slot is marked unavailable.
- Conversation state is stored in repository (`conversation_states`) to survive bot restarts.

## Development Setup

### Requirements

- Go 1.26+
- Docker + Docker Compose (optional for local development)
- Telegram bot token from `@BotFather`

### Environment

Copy environment template:

```bash
cp .env.example .env
```

Main variables:

- `TOKEN` - Telegram bot token.
- `USERNAME` - bot username (without `@`).
- `COMPOSE_PROJECT_NAME` - unique compose project name for running multiple bots on one server.
- `POSTGRES_DB`, `POSTGRES_USER` - database name and user for Compose (see [.env.example](.env.example)).
- **Postgres password (Compose)** - put a single line in `secrets/postgres_password` (not in git). On deploy, GitHub Actions writes this file from the `VPS_POSTGRES_PASSWORD` secret.
- `DB_DSN` - optional full DSN for local/non-Compose runs. If unset, the bot builds a DSN from `DB_PASSWORD_FILE` (set automatically in Compose) plus `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`. If neither `DB_DSN` nor `DB_PASSWORD_FILE` is available, the bot uses in-memory storage.
- `REDIS_ADDR` - optional, e.g. `localhost:6379` or `redis:6379` in Compose; enables caching of specialty list pages.
- `APP_ENV`, `LOG_LEVEL`, `LOG_FORMAT` - runtime options.
- `CLINIC_REFUND_PARTIAL_WINDOW` - optional partial-refund window for booking cancel policy (Go duration, default `24h`).
- `CLINIC_REFUND_PARTIAL_PERCENT` - optional partial-refund percentage in range `0..100` (default `50`).

### Database migration

Apply SQL migrations from [migrations](migrations) before running with PostgreSQL.

### Run locally

```bash
make run
```

### Testing

Запуск всех тестов (как в CI):

```bash
go test ./...
```

Через Makefile:

```bash
make test
```

Полная локальная проверка перед релизом (форматирование, `vet`, `staticcheck`, тесты, `govulncheck`, сборка Docker-образа):

```bash
make preprod
```

В CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) выполняются `go test ./...`, `go vet ./...` и `docker build`. Тесты используют **in-memory** репозиторий, отдельная БД для `go test` не нужна.

Перед тегом и релизом локально имеет смысл прогнать **`make preprod`** и при необходимости проверить **`docker compose up --build`** (есть `secrets/postgres_password` и `.env`).

### Run with Docker

```bash
make docker-run
```

### Run with Docker Compose

```bash
make docker-compose-up
```

The default [docker-compose.yaml](docker-compose.yaml) starts **Redis**, **PostgreSQL** (with `healthcheck`), then the **bot** after Postgres is healthy. Set a unique **`COMPOSE_PROJECT_NAME`** in `.env` if several bots run on the same host (see [.env.example](.env.example)). Create **`secrets/postgres_password`** with the DB password (one line). Compose mounts it as a [secret](https://docs.docker.com/compose/how-tos/use-secrets/) into Postgres and the bot. Do not commit that file (see [.gitignore](.gitignore)).

Stop:

```bash
make docker-compose-down
```

## CI/CD and Deployment

Workflows in [`.github/workflows`](.github/workflows):

| Workflow | Trigger | What it does |
|----------|---------|----------------|
| `ci.yml` | PR / push to `main` | `go test`, `go vet`, `docker build` |
| `release.yml` | Push tag `v*` | Build and push `ghcr.io/alekslesik/telegram-bot-pooling-hard:<tag>` and `:latest`, then deploy to VPS |
| `deploy.yml` | Manual (**Actions → Deploy → Run workflow**) | Redeploy an existing tag (default `latest`) without a new release |

### VPS layout (multi-bot on one server)

Use a **separate directory per bot**, each with its own `.env`, `COMPOSE_PROJECT_NAME`, and `secrets/postgres_password`. Example for this project:

```bash
/opt/bots/telegram-bot-pooling-hard
```

On first deploy, create the folder and place **`.env`** on the server (token, username, `POSTGRES_*` — **not** the DB password). Each workflow run copies **`docker-compose.prod.yaml`** from the repo and writes **`secrets/postgres_password`** from **`VPS_POSTGRES_PASSWORD`**.

### Required GitHub secrets (release + deploy)

| Secret | Purpose |
|--------|---------|
| `VPS_HOST` | SSH host (IP or hostname) |
| `VPS_USER` | SSH user (e.g. `root`) |
| `VPS_SSH_KEY` | Private SSH key (full PEM) |
| `VPS_APP_PATH` | e.g. `/opt/bots/telegram-bot-pooling-hard` |
| `VPS_POSTGRES_PASSWORD` | DB password; written to `secrets/postgres_password` on the VPS |
| `GHCR_READ_USER` | Optional: for **private** GHCR images |
| `GHCR_READ_TOKEN` | Optional: PAT with `read:packages` |

If the GHCR image is **public**, login on the VPS is skipped when those two are empty.

### Release flow

1. В GitHub → **Settings → Secrets and variables → Actions** должны быть заданы секреты из таблицы выше (`VPS_*`, `VPS_POSTGRES_PASSWORD`, при необходимости `GHCR_*`).

2. На актуальном `main` создайте и отправьте аннотированный тег версии:

```bash
git tag -a v1.0.2 -m "Release v1.0.2"
git push origin v1.0.2
```

3. Workflow **Release** ([`.github/workflows/release.yml`](.github/workflows/release.yml)) соберёт образ, опубликует его в **GHCR** и выполнит деплой на VPS (`VPS_APP_PATH`). Прогресс смотрите во вкладке **Actions**.

4. При необходимости повторного выката того же тега без новой сборки — workflow **Deploy** (ручной запуск).

### Troubleshooting deploy (`manifest unknown`)

- **Wrong image name**: this repo publishes only **`ghcr.io/alekslesik/telegram-bot-pooling-hard:<tag>`**. On the VPS, ensure **`docker-compose.prod.yaml`** under `VPS_APP_PATH` matches the repo (each Release/Deploy run copies it from GitHub). Remove stale **`docker-compose.override.yaml`** or any hand-edited compose that points at a different GHCR package. Check: `docker compose -f docker-compose.prod.yaml config | grep image:`.

- **Tag missing in GHCR**: open the **Release** workflow run for your tag and confirm the **`image`** job succeeded. If it failed, fix the error and push a new tag (or re-run after fixing). The pull uses **`IMAGE_TAG`** from the tag name; the image must exist under **`telegram-bot-pooling-hard`**, not another package name.

The bot uses **long polling**; no public webhook URL is required.
