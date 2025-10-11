# Contributing to Valkey Auth Proxy

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/valkey-auth-proxy.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Test your changes
6. Commit with clear messages
7. Push to your fork
8. Submit a Pull Request

## Development Setup

### Prerequisites

- Go 1.25 or later
- Docker (for testing containerization)
- Access to GCP (for integration testing)

### First-Time Setup

```bash
# Clone the repository
git clone https://github.com/awasilyev/cloud-valkey-proxy.git
cd cloud-valkey-proxy

# Install development tools
make install-tools

# Setup git hooks (recommended)
make setup-hooks
```

### Building

```bash
# Build the binary
make build

# Or use the build script
./build.sh
```

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
go test -v -race -coverprofile=coverage.txt ./...

# Quick tests (for pre-commit)
make test-short
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run go vet
make vet

# Run all checks (format, vet, lint, test)
make check
```

### Pre-commit Hooks

Pre-commit hooks are automatically installed with `make setup-hooks`. They run:

1. **gofmt** - Ensures code is properly formatted
2. **go vet** - Checks for common mistakes
3. **go mod tidy** - Ensures dependencies are clean
4. **Unit tests** - Runs all tests with race detection
5. **golangci-lint** - Comprehensive linting (if installed)

To skip hooks for a specific commit (not recommended):
```bash
git commit --no-verify
```

### Running Locally

```bash
# Set required environment variables
export VALKEY_INSTANCE_NAME="projects/YOUR_PROJECT/locations/YOUR_LOCATION/instances/YOUR_INSTANCE"

# Run the proxy
./cloud-valkey-proxy -verbose=true
```

## Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions focused and small

### Linting

```bash
# Run linter
make lint

# Or directly
golangci-lint run ./...

# Use helper script
./scripts/lint.sh
```

### Helper Scripts

Located in the `scripts/` directory:

- `setup-hooks.sh` - Install git hooks
- `install-tools.sh` - Install development tools
- `fmt.sh` - Format all Go code
- `lint.sh` - Run golangci-lint
- `test.sh` - Run tests with coverage

## Testing

- Write unit tests for new functionality
- Maintain or improve test coverage
- Test edge cases and error conditions
- Use table-driven tests where appropriate

Example test structure:
```go
func TestFunction(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"case1", "input1", "output1"},
        {"case2", "input2", "output2"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := Function(tt.input)
            if result != tt.expected {
                t.Errorf("expected %s, got %s", tt.expected, result)
            }
        })
    }
}
```

## Documentation

- Update README.md for user-facing changes
- Update CHANGELOG.md following [Keep a Changelog](https://keepachangelog.com/)
- Add inline comments for complex logic
- Update examples if adding new features

## Pull Request Process

1. **Title**: Use a clear, descriptive title
   - ✅ "Add support for custom port ranges"
   - ❌ "Update code"

2. **Description**: Include:
   - What changes were made
   - Why the changes were necessary
   - Any breaking changes
   - Related issues (if applicable)

3. **Testing**: Describe how you tested the changes

4. **Documentation**: Update relevant documentation

5. **Commits**: Keep commits atomic and well-described

## Commit Message Guidelines

Follow the conventional commits specification:

```
type(scope): subject

body

footer
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

Examples:
```
feat(proxy): add connection pooling support

Implements connection pooling to improve performance
for high-traffic scenarios.

Closes #123
```

```
fix(auth): handle token refresh on expiration

Previously, expired tokens would cause authentication
failures. Now tokens are refreshed automatically.
```

## Code Review

All submissions require code review. We follow these principles:

- Be respectful and constructive
- Focus on the code, not the person
- Explain reasoning for suggestions
- Accept that there are multiple valid approaches

## Performance Considerations

This proxy is designed for minimal latency. When contributing:

- Avoid unnecessary allocations
- Use buffered I/O where appropriate
- Profile changes if they affect the critical path
- Consider TCP optimization options
- Benchmark performance-critical code

## Security

- Never commit credentials or secrets
- Report security issues privately (see SECURITY.md)
- Follow secure coding practices
- Validate all user input
- Handle errors appropriately

## Questions?

Feel free to open an issue for:
- Questions about contributing
- Feature proposals
- Bug reports
- General discussions

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

