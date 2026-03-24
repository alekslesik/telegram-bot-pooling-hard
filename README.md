# telegram-bot-pooling-middle

Level 2 Telegram bot template for service booking scenarios (hair salon, dentist, consultations).

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

The project already includes a base Go bot scaffold, tests, Docker packaging, and CI/CD workflows.  
The repository now includes an MVP booking wizard with persistent conversation state.

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
- `APP_ENV`, `LOG_LEVEL`, `LOG_FORMAT` - runtime options.

### Database migration

Apply SQL migrations from [migrations](migrations) before running with PostgreSQL.

### Run locally

```bash
make run
```

### Run tests

```bash
make test
```

### Run with Docker

```bash
make docker-run
```

### Run with Docker Compose

```bash
make docker-compose-up
```

The default [docker-compose.yaml](docker-compose.yaml) starts **PostgreSQL** with a `healthcheck` and starts the **bot only after the database is healthy** (`depends_on: condition: service_healthy`). Create **`secrets/postgres_password`** with the DB password (one line, no newline required). Compose mounts it as a [secret](https://docs.docker.com/compose/how-tos/use-secrets/) into Postgres (`POSTGRES_PASSWORD_FILE`) and the bot (`DB_PASSWORD_FILE`). Do not commit that file (see [.gitignore](.gitignore)).

Stop:

```bash
make docker-compose-down
```

## CI/CD and Deployment

The repository contains GitHub Actions workflows for:

- `ci.yml` - build, lint, test, vulnerability checks, docker build.
- `release.yml` - build and push image to GHCR, then deploy to VPS.
- `deploy.yml` - manual/deprecated SSH deploy helper.

### VPS layout (multi-bot safe)

Recommended path for this project:

```bash
/opt/bots/telegram-bot-pooling-middle
```

Place `.env` in this folder on the server (token, username, `POSTGRES_*` names — **not** the DB password).  
`docker-compose.prod.yaml` is uploaded during release deploy.

The deploy job writes **`secrets/postgres_password`** on the VPS from **`VPS_POSTGRES_PASSWORD`** so the password never lives in the repo.

### Required GitHub secrets

- `VPS_HOST`
- `VPS_USER`
- `VPS_SSH_KEY`
- `VPS_APP_PATH` (set to `/opt/bots/telegram-bot-pooling-middle`)
- `VPS_POSTGRES_PASSWORD` (database password; synced to `secrets/postgres_password` on the server each deploy)
- `GHCR_READ_USER`
- `GHCR_READ_TOKEN`

### Release flow

1. Create and push a tag:

```bash
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

2. Publish a GitHub Release for this tag.
3. Workflow builds image `ghcr.io/<owner>/<repo>:vX.Y.Z` and deploys it to VPS.

The bot runs in long polling mode, so no public webhook endpoint is required.
