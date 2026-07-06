# Contributing

## Setup

```bash
cp .env.example .env
make docker-up
make run
```

## Workflow

1. Branch from `dev`
2. Write code + tests
3. `make test` must pass
4. Open PR to `dev`

## Commit Style

Conventional commits: `feat:`, `fix:`, `chore:`, `docs:`, `test:`, `refactor:`

## Code

- Follow existing patterns in the file you're editing
- Tests use SQLite `:memory:` — zero external deps
- No float64 for money — use `shopspring/decimal`
