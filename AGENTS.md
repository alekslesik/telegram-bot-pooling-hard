# Agent Working Notes

This file captures how AI coding agents should operate in this repository.

## Engineering Priorities

- Think like a backend architect: reliability, security, scalability, and performance first.
- Keep architecture clean: `transport -> service -> repository`.
- Prefer safe, auditable defaults (no hardcoded secrets, explicit migrations, conservative rollout).
- Preserve backward compatibility unless a breaking change is explicitly requested.

## Task Execution Scope

Always use a feature branch, commit with Conventional Commits, push, and provide a PR link.

Then choose the flow by change type:

### Code/runtime changes

- Run:
  - `make preprod`
  - `make docker-compose-up` (verify startup and logs)
  - `make docker-compose-down`
- After push/PR:
  - delete local feature branch
  - sync local `main` (`git fetch --prune origin` + `git pull --ff-only`)
  - create and push a new annotated tag

### Docs/meta changes (no runtime impact)

- Use lightweight flow:
  - commit + push + PR
  - delete local feature branch
  - sync local `main`
- Skip runtime checks and skip release tag unless explicitly requested.

## Git Safety

- Never commit or push directly to `main`.
- Delete only local branches unless explicitly instructed otherwise.
- Avoid destructive git operations unless explicitly requested.
