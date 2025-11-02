# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`etcd-secret-reader` is a CLI tool that reads and decrypts Kubernetes secrets from encrypted etcd backup snapshots without requiring a full cluster restore. It uses official etcd MVCC libraries and Go crypto libraries to properly decode etcd v3 snapshots and decrypt secrets encrypted with AES-CBC. The tool supports:
- Both standard Kubernetes (`/registry/secrets/`) and OpenShift (`/kubernetes.io/secrets/`) secret storage paths
- Both JSON and protobuf-encoded secret formats (OpenShift uses protobuf)

## Build & Test Commands

### Building
```bash
# Build for current platform
make build

# Build for all platforms (Linux, macOS, Windows)
make build-all

# Install to /usr/local/bin
make install
```

### Testing
```bash
# Run all tests with race detection and coverage
make test

# Run specific package tests
go test -v ./pkg/decrypt/
go test -v ./pkg/etcdreader/
go test -v ./test/

# Run specific test by name
go test -v ./pkg/decrypt/ -run TestDecrypt

# View coverage report
go test -coverprofile=coverage.txt -covermode=atomic ./...
go tool cover -html=coverage.txt
```

### Code Quality
```bash
# Run all checks (format, vet, tests)
make check

# Format code
make fmt

# Run go vet
make vet
```

## Code Architecture

### Package Structure

The codebase follows a clean three-layer architecture:

1. **cmd/etcd-secret-reader/main.go** - CLI entry point
   - Parses command-line flags
   - Coordinates between reader and decryptor packages
   - Handles display formatting for secrets
   - Decodes both JSON and protobuf-encoded secrets using Kubernetes API libraries

2. **pkg/etcdreader/reader.go** - etcd snapshot reading layer
   - Opens BBolt databases in read-only mode
   - Uses official etcd MVCC libraries to decode revision keys and protobuf values
   - Reads from the "key" bucket where etcd v3 stores MVCC-encoded data
   - Lists secrets under `/registry/secrets/` and `/kubernetes.io/secrets/` prefixes
   - Handles tombstones (deleted keys) correctly
   - No awareness of encryption - returns raw decrypted bytes from MVCC values

3. **pkg/decrypt/aescbc.go** - Decryption layer
   - Handles AES-CBC decryption with PKCS#7 padding
   - Parses k8s encryption format: `k8s:enc:aescbc:v1:<keyName>:<IV><encrypted-data>`
   - Supports plaintext (identity provider) passthrough
   - No awareness of etcd storage - purely crypto operations

### Key Technical Details

**Kubernetes Secret Storage Path:**
- Standard Kubernetes: `/registry/secrets/<namespace>/<secret-name>`
- OpenShift: `/kubernetes.io/secrets/<namespace>/<secret-name>`
- The etcdreader package supports both formats automatically

**MVCC Encoding (Critical Understanding):**
etcd v3 uses Multi-Version Concurrency Control (MVCC) to store data:
- BBolt keys are **revision numbers** (17 bytes: 8-byte main rev + '_' + 8-byte sub rev), NOT the actual key names
- BBolt values are **protobuf-encoded KeyValue messages** containing the actual key name and value
- To find a key, you must iterate through all entries and unmarshal the protobuf to check the key name
- Tombstones (deleted keys) are marked by an extra byte (18 bytes instead of 17)
- Example: To find `/registry/secrets/default/my-secret`, iterate all entries, unmarshal each value's protobuf, check if `kv.Key == "/registry/secrets/default/my-secret"`

**Secret Encoding:**
After decryption (if encrypted), secrets can be in two formats:
- **JSON** (standard Kubernetes): Regular JSON object with `kind`, `metadata`, `data` fields
- **Protobuf** (OpenShift): Binary format starting with `k8s\x00`, decoded using `k8s.io/api/core/v1` and `k8s.io/apimachinery/pkg/runtime/serializer`

**Encryption Format:**
When Kubernetes encrypts secrets with AES-CBC, the format is:
```
k8s:enc:aescbc:v1:<keyName>:<IV (16 bytes)><encrypted-data>
```
- First 16 bytes after keyName are the initialization vector (IV)
- Remaining bytes are the AES-CBC ciphertext with PKCS#7 padding
- Decryption requires the exact 32-byte key used during encryption
- After decryption, the result can be either JSON or protobuf-encoded

**BBolt Database:**
- etcd v3 snapshots are BBolt databases (formerly BoltDB)
- Data is stored in a bucket named "key" (accessed via `buckets.Key.Name()`)
- Direct key lookups don't work - must use MVCC decoding as described above

### Testing Strategy

The project has >90% test coverage with three test levels:

1. **Unit tests** (pkg/*/\*_test.go):
   - Test individual functions with table-driven tests
   - Use in-memory fixtures and temporary files
   - Cover edge cases (wrong keys, invalid padding, truncated data)

2. **Integration tests** (test/integration_test.go):
   - End-to-end workflow: create snapshot → encrypt secret → decrypt secret
   - Multi-namespace scenarios
   - Error handling with wrong keys

3. **Test data isolation**:
   - Uses `t.TempDir()` for BBolt databases to prevent lock conflicts
   - Generates random encryption keys per test for independence
   - **IMPORTANT**: Test helpers must use MVCC encoding (see `createTestSnapshot()` in reader_test.go)
   - Keys must be stored as protobuf `mvccpb.KeyValue` with proper revision bytes

### Common Patterns

**Reading secrets:**
```go
reader, _ := etcdreader.NewReader(snapshotPath)
defer reader.Close()
data, _ := reader.Get("/registry/secrets/default/my-secret")
```

**Decrypting:**
```go
decryptor, _ := decrypt.NewAESCBCDecryptor(keyBytes, "key1")
plaintext, _ := decryptor.Decrypt(encryptedData)
```

**Listing secrets:**
```go
secrets, _ := reader.ListSecrets()  // Returns all /registry/secrets/* keys
```

## Development Guidelines

### When Modifying Decryption Code
- Always test with both valid and invalid padding scenarios
- Test key name mismatches separately from decryption failures
- Verify plaintext passthrough still works (identity provider)
- Maintain compatibility with the k8s:enc:aescbc:v1 format

### When Modifying Reader Code
- **CRITICAL**: etcd stores keys using MVCC encoding - BBolt keys are revision numbers, not actual key names
- Always unmarshal protobuf values to get the actual key name: `var kv mvccpb.KeyValue; kv.Unmarshal(value)`
- Check tombstones with `len(k) == 18` (markedRevBytesLen) vs normal keys `len(k) == 17` (revBytesLen)
- Remember BBolt cursors are only valid during transactions
- Always copy data out of the transaction with `copy()`
- Use read-only mode for all snapshot access via `&bolt.Options{ReadOnly: true}`
- Use `buckets.Key.Name()` to get the correct bucket name, not hardcoded `[]byte("key")`

### Binary Output Handling
- Use `safePrintKey()` pattern (main.go:32-42) when printing potentially binary keys
- Check if strings are printable before outputting to console
- This prevents terminal corruption from control characters

### Error Messages
- Include context in error wrapping: `fmt.Errorf("failed to X: %w", err)`
- Provide helpful troubleshooting tips (see `--list-all` suggestion in main.go:123)
- Distinguish between different failure modes (wrong key vs. missing key vs. invalid format)

## Security Considerations

- Never log or display encryption keys
- Warn users before committing `.env` or key files
- The tool operates in read-only mode on snapshots
- AES-CBC is considered less secure than AES-GCM; this is a Kubernetes limitation, not a tool choice

## Nix Development Environment

This project uses Nix flakes for reproducible development environments. The flake provides Go, gitleaks, golangci-lint, and other tools. Enter the environment with:
```bash
nix develop
```
