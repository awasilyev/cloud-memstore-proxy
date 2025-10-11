# Pre-commit Setup Guide

This project includes automated pre-commit checks to ensure code quality.

## Quick Setup

```bash
# 1. Install development tools
make install-tools

# 2. Setup git hooks
make setup-hooks

# 3. You're ready! Hooks will run automatically on commit
```

## What Gets Checked

Every time you commit, these checks run automatically:

### 1. Code Formatting ✨
- Ensures all Go code is properly formatted with `gofmt`
- Fails if any files need formatting
- **Fix**: Run `make fmt` or `go fmt ./...`

### 2. Go Vet 🔍
- Runs static analysis to catch common mistakes
- Checks for suspicious constructs
- **Fix**: Address the issues reported by `go vet`

### 3. Dependency Hygiene 📦
- Ensures `go.mod` and `go.sum` are tidy
- Removes unused dependencies
- **Fix**: Run `go mod tidy` and commit changes

### 4. Unit Tests 🧪
- Runs all tests with race detection
- Uses `-short` flag for faster execution
- **Fix**: Fix failing tests before committing

### 5. Linting 🔧
- Runs comprehensive code analysis (if golangci-lint is installed)
- Checks for code smells, bugs, and style issues
- **Fix**: Address linter warnings

## Manual Commands

Run any check manually:

```bash
# Format code
make fmt

# Run linter
make lint

# Run go vet
make vet

# Run tests
make test

# Run ALL checks
make check
```

## Helper Scripts

Individual scripts in `scripts/` directory:

```bash
./scripts/fmt.sh          # Format code
./scripts/lint.sh         # Run linter
./scripts/test.sh         # Run tests with coverage
./scripts/install-tools.sh # Install dev tools
./scripts/setup-hooks.sh   # Setup git hooks
```

## Skipping Pre-commit Checks

**⚠️ Not recommended**, but sometimes necessary:

```bash
git commit --no-verify -m "Your message"
```

Use this only for:
- Work-in-progress commits on feature branches
- Emergency hotfixes (but fix issues ASAP)

## Troubleshooting

### Hook not running

```bash
# Reinstall hooks
./scripts/setup-hooks.sh

# Check if executable
chmod +x .githooks/pre-commit
```

### golangci-lint not found

```bash
# Install it
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Or use our script
./scripts/install-tools.sh
```

### Tests failing

```bash
# Run tests to see details
make test

# Run with verbose output
go test -v ./...

# Run specific test
go test -v -run TestName ./pkg/...
```

### Code not formatted

```bash
# Auto-format all code
make fmt

# Check what needs formatting
gofmt -l .
```

## IDE Integration

### VS Code

Add to `.vscode/settings.json`:

```json
{
  "go.formatTool": "goimports",
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "workspace",
  "[go]": {
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": true
    }
  }
}
```

### GoLand/IntelliJ

1. Preferences → Tools → File Watchers
2. Add Go fmt file watcher
3. Enable "Run on Save"

## CI/CD Integration

The same checks run in GitHub Actions:

- **PR Workflow**: All quality checks on every pull request
- **Build Workflow**: Tests and linting on push
- **Release Workflow**: Full checks before release

## Benefits

✅ Catch issues early (before push)  
✅ Maintain consistent code style  
✅ Prevent breaking changes  
✅ Faster code reviews  
✅ Better code quality  

## Alternative: Pre-commit Framework

For more advanced setups, use the Python-based pre-commit framework:

```bash
# Install (requires Python)
pip install pre-commit

# Install hooks from .pre-commit-config.yaml
pre-commit install

# Run manually
pre-commit run --all-files
```

Configuration is in `.pre-commit-config.yaml`.

## Summary

| Check | Command | Auto-fix |
|-------|---------|----------|
| Format | `make fmt` | ✅ Yes |
| Vet | `make vet` | ❌ Manual |
| Lint | `make lint` | ⚠️ Some |
| Test | `make test` | ❌ Manual |
| All | `make check` | ⚠️ Partial |

**Remember**: The pre-commit hook is there to help you, not slow you down. If you find it too strict, we can adjust the checks together!

