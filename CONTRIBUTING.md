# Contributing to TokenSentinel

## Quick Start

```bash
git clone https://github.com/Tejas163/TokenSentinel.git
cd TokenSentinel/app
go run .
```

Open http://localhost:8080, upload a billing CSV, and get your report.

## Project Structure

```
TokenSentinel/
├── app/          # Single Go binary — the entire product
│   ├── main.go       # HTTP server, CSV parsing, PDF generation
│   ├── engine/       # Prescriptive savings engine
│   ├── landing.html  # Landing page
│   ├── report.html   # Report page
│   └── main_test.go  # Tests
├── demo/         # (reserved)
└── README.md
```

## Development Workflow

### 1. Make Changes
- Follow existing code style and patterns
- Update tests where applicable
- Run `go test ./...` in the `app/` directory

### 2. Build
```bash
cd app
go build -o tokensentinel .
```

### 3. Test
```bash
cd app
go test ./... -v
```

### 4. Commit
```
type(scope): brief description

- Bullet points for details if needed
- Reference issues: Fixes #123
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `security`

### 5. Push and PR
```bash
git push origin feat/your-feature-name
# Create PR on GitHub
```

## Code Guidelines

### Go
- Use `gofmt` before committing
- Use `log/slog` for structured logging
- Use `net/http` standard library patterns
- Keep `main.go` focused on HTTP routing — business logic goes in `engine/`

### HTML/CSS/JS
- Keep CSS in a single `<style>` block per page
- Use vanilla JS (no framework dependencies)
- Support mobile via `@media (max-width: 640px)` queries
- Match the dark theme color palette

## Testing

```bash
# All tests
cd app && go test ./... -v
```

## Adding a New Model

1. Add model info to `app/engine/models.go` `ModelCatalog`
2. Optionally add equivalence mappings to `ModelEquivalence`
3. Add GPU reference pricing if applicable

## Adding a New CSV Format

1. Add header aliases to `knownHeaders` in `app/main.go`
2. Add test cases to `TestDetectColumns` in `app/main_test.go`

## Security

- Never commit secrets or API keys
- Use environment variables for all configuration
- The binary processes all data locally — no outbound calls except optional API key imports

## Getting Help

- Open a [GitHub issue](https://github.com/Tejas163/TokenSentinel/issues)
