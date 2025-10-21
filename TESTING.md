# Testing Guide

This document describes the testing strategy and how to run tests for the etcd-secret-reader project.

## Test Structure

The project includes comprehensive unit and integration tests:

```
etcd-secret-reader/
├── pkg/
│   ├── decrypt/
│   │   ├── aescbc.go
│   │   └── aescbc_test.go        # Unit tests for decryption
│   └── etcdreader/
│       ├── reader.go
│       └── reader_test.go         # Unit tests for etcd reader
└── test/
    └── integration_test.go        # Integration tests
```

## Running Tests

### Run All Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -coverprofile=coverage.txt -covermode=atomic ./...

# Run with race detection
go test -race ./...

# Run with coverage and race detection (recommended)
go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
```

### Run Specific Test Packages

```bash
# Run only decrypt package tests
go test -v ./pkg/decrypt/

# Run only etcd reader package tests
go test -v ./pkg/etcdreader/

# Run only integration tests
go test -v ./test/
```

### Run Specific Tests

```bash
# Run a specific test by name
go test -v ./pkg/decrypt/ -run TestDecrypt

# Run tests matching a pattern
go test -v ./... -run TestAESCBC
```

### Using Make

```bash
# Run all tests
make test

# Run tests with verbose output
make test TESTFLAGS="-v"

# Run specific test package
make test TESTFLAGS="-v ./pkg/decrypt/"
```

## Test Coverage

Current test coverage:

- **decrypt package**: 92.6% coverage
- **etcdreader package**: 90.0% coverage

### View Coverage Report

```bash
# Generate coverage report
go test -coverprofile=coverage.txt -covermode=atomic ./...

# View coverage in terminal
go tool cover -func=coverage.txt

# Generate HTML coverage report
go tool cover -html=coverage.txt -o coverage.html

# Open HTML report in browser
open coverage.html  # macOS
xdg-open coverage.html  # Linux
```

## Test Categories

### Unit Tests

#### 1. AES-CBC Decryption Tests (`pkg/decrypt/aescbc_test.go`)

Tests the core encryption/decryption functionality:

- **TestNewAESCBCDecryptor**: Decryptor initialization with various key sizes
- **TestRemovePKCS7Padding**: PKCS#7 padding removal (valid and invalid cases)
- **TestIsEncrypted**: Detection of encrypted vs plaintext data
- **TestParseEncryptionPrefix**: Parsing k8s encryption prefix format
- **TestDecrypt**: Decryption with various scenarios:
  - Successful decryption with correct key
  - Failure with wrong key
  - Failure with wrong key name
  - Handling plaintext (identity provider) secrets
  - Invalid prefix handling
  - Truncated data handling
- **TestRoundTrip**: Encrypt/decrypt round-trip verification
- **TestBase64KeyHandling**: Base64 key encoding/decoding

#### 2. Etcd Reader Tests (`pkg/etcdreader/reader_test.go`)

Tests the etcd snapshot reading functionality:

- **TestNewReader**: Reader initialization (valid/invalid snapshots)
- **TestReaderClose**: Proper resource cleanup
- **TestReaderGet**: Reading specific keys from snapshot
- **TestReaderListSecrets**: Listing all secrets
- **TestReaderListSecretsEmpty**: Handling snapshots with no secrets
- **TestReaderListAll**: Listing all keys (not just secrets)
- **TestReaderWithRealSecretFormat**: Handling real Kubernetes secret formats

### Integration Tests

#### End-to-End Tests (`test/integration_test.go`)

Tests the complete workflow:

- **TestEndToEndEncryptionDecryption**: Full encrypt/decrypt cycle
  - Creates snapshot with encrypted secrets
  - Lists all secrets
  - Decrypts and verifies each secret
  - Validates Kubernetes Secret structure

- **TestDecryptionWithWrongKey**: Error handling for incorrect keys

- **TestPlaintextSecretHandling**: Handling unencrypted (identity provider) secrets

- **TestMultipleSecretsInDifferentNamespaces**: Multi-namespace scenario
  - Creates secrets in multiple namespaces
  - Verifies all are listed and can be decrypted
  - Tests namespace isolation

## Benchmarks

Run performance benchmarks:

```bash
# Run all benchmarks
go test -bench=. ./...

# Run specific benchmark
go test -bench=BenchmarkDecrypt ./pkg/decrypt/

# Run with memory allocation stats
go test -bench=. -benchmem ./...

# Run benchmarks with CPU profiling
go test -bench=. -cpuprofile=cpu.prof ./...
```

Example benchmark results:
```
BenchmarkDecrypt-8                     500000    3542 ns/op
BenchmarkReaderGet-8                  1000000    1123 ns/op
BenchmarkReaderListSecrets-8           100000   11234 ns/op
BenchmarkEndToEndDecryption-8          200000    8765 ns/op
```

## Writing New Tests

### Test Naming Conventions

- Test functions: `Test<FunctionName>` (e.g., `TestDecrypt`)
- Benchmark functions: `Benchmark<FunctionName>` (e.g., `BenchmarkDecrypt`)
- Table-driven tests: Use descriptive test names

### Example Test Structure

```go
func TestMyFunction(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        want      string
        wantError bool
    }{
        {
            name:      "Valid input",
            input:     "test",
            want:      "result",
            wantError: false,
        },
        {
            name:      "Invalid input",
            input:     "",
            want:      "",
            wantError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MyFunction(tt.input)
            if tt.wantError {
                if err == nil {
                    t.Errorf("expected error, got nil")
                }
                return
            }
            if err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

## CI/CD Integration

Tests run automatically in GitHub Actions:

- On every push to `main`/`master` and `claude/**` branches
- On every pull request
- Before creating releases

See `.github/workflows/test.yml` for the CI configuration.

## Test Data

The tests use:

- **In-memory test fixtures**: Created programmatically in each test
- **Temporary files**: Uses `t.TempDir()` for isolated test databases
- **Random encryption keys**: Generated for each test run to ensure independence

## Troubleshooting

### Tests Fail with "database locked"

BBolt requires exclusive access to database files. Ensure:
- Previous test runs have cleaned up
- No other processes are accessing test files

Solution: Tests use `t.TempDir()` to create isolated temporary directories.

### Tests Fail with "invalid padding"

This usually indicates:
- Wrong encryption key
- Corrupted encrypted data
- Mismatched encryption/decryption parameters

Check that test keys match between encryption and decryption.

### Race Condition Warnings

Run with race detector:
```bash
go test -race ./...
```

Fix any reported races before committing.

## Best Practices

1. **Always run tests before committing**:
   ```bash
   make check  # Runs fmt, vet, and tests
   ```

2. **Write tests for new features**: Aim for >90% coverage

3. **Use table-driven tests**: For testing multiple scenarios

4. **Test error cases**: Don't just test the happy path

5. **Clean up resources**: Use `defer` for cleanup operations

6. **Use test helpers**: Extract common setup code to helper functions

7. **Make tests deterministic**: Don't rely on timing or external state

## Coverage Goals

- **Overall project**: >85% coverage
- **Critical packages** (decrypt, etcdreader): >90% coverage
- **New code**: 100% coverage of new functions

## Continuous Improvement

- Review test failures in CI
- Add tests for reported bugs
- Refactor tests when they become hard to maintain
- Update this document when test structure changes
