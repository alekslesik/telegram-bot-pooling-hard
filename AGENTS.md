# Agent Working Notes

This file captures how AI coding agents should operate in this repository.

## Engineering Priorities

- Think like a backend architect: reliability, security, scalability, and performance first.
- Keep architecture clean: `transport -> service -> repository`.
- Prefer safe, auditable defaults (no hardcoded secrets, explicit migrations, conservative rollout).
- Preserve backward compatibility unless a breaking change is explicitly requested.

## Task Execution Scope

Always use a feature branch, commit with Conventional Commits, push, and provide a PR link.

PR ownership preference:
- The agent prepares branches/commits/PRs, but does **not** merge PRs unless the user explicitly asks.
- Each PR body should include a clear changelog and a `Release notes draft` section for GitHub Releases.
- Before opening every PR, run `make preprod`.
- After local runtime verification, always stop started containers using `make docker-compose-down`.

Then choose the flow by change type:

### Code/runtime changes

- Run:
  - `make preprod`
  - `make docker-compose-up` (verify startup and logs)
  - `make docker-compose-down`
- After push/PR:
  - wait for GitHub Actions CI to pass (green)
  - merge the PR only after explicit user confirmation/request
  - delete local feature branch
  - sync local `main` (`git fetch --prune origin` + `git pull --ff-only`)
  - create and push a new annotated tag

### Docs/meta changes (no runtime impact)

- Use lightweight flow:
  - run `make preprod`
  - commit + push + PR
  - wait for GitHub Actions CI to pass (green)
  - merge the PR only after explicit user confirmation/request
  - delete local feature branch
  - sync local `main`
- If any containers were started during checks, run `make docker-compose-down`.
- Skip local runtime checks and skip release tag unless explicitly requested.

## Git Safety

- Never commit or push directly to `main`.
- Delete only local branches unless explicitly instructed otherwise.
- Avoid destructive git operations unless explicitly requested.
