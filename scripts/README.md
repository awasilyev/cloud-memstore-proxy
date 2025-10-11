# Development Scripts

Helper scripts for development and CI/CD.

## Setup Scripts

### `install-tools.sh`
Installs required development tools:
- golangci-lint (linter)
- goimports (import formatter)
- Information about pre-commit framework

```bash
./scripts/install-tools.sh
```

### `setup-hooks.sh`
Installs git pre-commit hooks that run automatically before each commit.

```bash
./scripts/setup-hooks.sh
```

Or use make:
```bash
make setup-hooks
```

## Code Quality Scripts

### `fmt.sh`
Formats all Go code using `gofmt` and `goimports` (if available).

```bash
./scripts/fmt.sh
```

### `lint.sh`
Runs golangci-lint with proper timeout settings.

```bash
./scripts/lint.sh
```

### `test.sh`
Runs all tests with race detection and generates coverage report.

```bash
./scripts/test.sh

# View coverage in browser
go tool cover -html=coverage.out
```

## Pre-commit Hook

The pre-commit hook (`.githooks/pre-commit`) runs automatically before each commit and checks:

1. ✅ Code formatting (gofmt)
2. ✅ Go vet (static analysis)
3. ✅ Go mod tidy (dependency hygiene)
4. ✅ Unit tests (with race detection)
5. ✅ Linting (if golangci-lint is installed)

### Skipping Pre-commit Checks

In rare cases, you may need to skip the pre-commit checks:

```bash
git commit --no-verify -m "Your commit message"
```

**Note:** This is not recommended for regular development.

## Makefile Targets

All scripts can also be run via make:

```bash
make install-tools  # Install development tools
make setup-hooks    # Setup git hooks
make fmt           # Format code
make lint          # Run linter
make vet           # Run go vet
make test          # Run tests with coverage
make check         # Run all checks (fmt, vet, lint, test)
```

## CI/CD Integration

These scripts are also used in GitHub Actions workflows:

- `.github/workflows/build.yaml` - Uses test and lint scripts
- `.github/workflows/pr.yaml` - Uses all quality checks
- `.github/workflows/release.yaml` - Uses build and test

## Troubleshooting

### golangci-lint not found

Install it:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Or use the install script:
```bash
./scripts/install-tools.sh
```

### Pre-commit hook not running

Make sure it's executable and configured:
```bash
chmod +x .githooks/pre-commit
./scripts/setup-hooks.sh
```

### Tests failing in pre-commit

The pre-commit runs tests with race detection. Fix the issues or temporarily skip:
```bash
git commit --no-verify
```

Then fix the issues and amend:
```bash
# Fix the issues
make test

# Amend the commit
git commit --amend --no-edit
```

