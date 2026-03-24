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
The next iterations should implement Level 2 business features on top of this foundation.

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
- `APP_ENV`, `LOG_LEVEL`, `LOG_FORMAT` - runtime options.

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

Place `.env` in this folder on the server.  
`docker-compose.prod.yaml` is uploaded during release deploy.

### Required GitHub secrets

- `VPS_HOST`
- `VPS_USER`
- `VPS_SSH_KEY`
- `VPS_APP_PATH` (set to `/opt/bots/telegram-bot-pooling-middle`)
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
