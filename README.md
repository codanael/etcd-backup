# etcd-secret-reader

Read and decrypt Kubernetes secrets from encrypted etcd snapshots without restoring the cluster.

## Quick Start

```bash
# Download from releases
wget https://github.com/codanael/etcd-backup/releases/latest/download/etcd-secret-reader-linux-amd64.tar.gz
tar -xzf etcd-secret-reader-linux-amd64.tar.gz
chmod +x etcd-secret-reader

# List all secrets
./etcd-secret-reader --snapshot=snapshot.db --list

# Decrypt a specific secret
./etcd-secret-reader --snapshot=snapshot.db --namespace=default --name=my-secret --key=<base64-key>
```

## Installation

**Pre-built binaries**: Download from [Releases](../../releases) for Linux, macOS, or Windows.

**Build from source**:
```bash
make build          # Current platform
make build-all      # All platforms
make install        # Install to /usr/local/bin
```

**Using Go**:
```bash
go build -o etcd-secret-reader ./cmd/etcd-secret-reader
```

## Usage

### Basic Commands

```bash
# List secrets
etcd-secret-reader --snapshot=snapshot.db --list

# Decrypt specific secret
etcd-secret-reader --snapshot=snapshot.db --namespace=default --name=my-secret --key=<base64-key>

# Decrypt all secrets
etcd-secret-reader --snapshot=snapshot.db --key=<base64-key>

# Debug: list all keys
etcd-secret-reader --snapshot=snapshot.db --list-all
```

### Flags

| Flag | Description | Required |
|------|-------------|----------|
| `--snapshot` | Path to etcd snapshot file | Yes |
| `--key` | Base64-encoded 32-byte AES-CBC key | For decryption |
| `--namespace` | Kubernetes namespace | No |
| `--name` | Secret name | No |
| `--key-name` | Encryption key name (default: "key1") | No |
| `--list` | List all secrets without decrypting | No |
| `--list-all` | List all keys (debugging) | No |

## Getting Your Encryption Key

From your cluster's EncryptionConfiguration (`/etc/kubernetes/encryption-config.yaml`):

```bash
grep -A1 "secret:" /etc/kubernetes/encryption-config.yaml | tail -1 | awk '{print $2}'
```

The key must be the base64-encoded 32-byte key from your cluster's configuration:

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: <BASE64-ENCODED-32-BYTE-KEY>  # Use this value
```

## Creating an etcd Snapshot

```bash
ETCDCTL_API=3 etcdctl snapshot save snapshot.db \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key
```

## Example

```bash
# Create a test secret
kubectl create secret generic test-secret --from-literal=password=secretpass

# Take snapshot
ETCDCTL_API=3 etcdctl snapshot save snapshot.db

# List secrets
./etcd-secret-reader --snapshot=snapshot.db --list

# Decrypt
./etcd-secret-reader --snapshot=snapshot.db --namespace=default --name=test-secret --key=<your-key>
```

Output:
```
Secret: default/test-secret
Type: Opaque
Data:
  password: secretpass
```

## Troubleshooting

**No secrets found?**

1. Run `--list-all` to see all keys in the snapshot
2. Verify snapshot is from etcd v3: `file snapshot.db` (should show "data")
3. Confirm snapshot is from the control plane node
4. Check if secrets use a different storage path or encryption provider

**Decryption fails?**

- Verify you're using the correct base64-encoded key from your cluster's EncryptionConfiguration
- Ensure the `--key-name` matches your configuration (default: "key1")
- Check that secrets were encrypted with AES-CBC (not AES-GCM or KMS)

## How It Works

This tool:
1. Opens etcd snapshots (BBolt database) in read-only mode
2. Uses MVCC libraries to decode etcd v3 storage format
3. Decrypts secrets using AES-CBC with the provided key
4. Supports both standard Kubernetes (`/registry/secrets/`) and OpenShift (`/kubernetes.io/secrets/`) paths
5. Handles both JSON and protobuf-encoded secrets

**Encryption format**: `k8s:enc:aescbc:v1:<keyName>:<IV><encrypted-data>`

## Security

- Never commit encryption keys to version control
- Restrict access to snapshot files and keys
- AES-CBC is less secure than AES-GCM (Kubernetes limitation)
- Use for emergency recovery only

## Testing

Comprehensive test suite with >90% coverage:

```bash
make test        # Run all tests
make check       # Run fmt, vet, and tests
go test -race ./...  # With race detection
```

Coverage: **decrypt** 92.6%, **etcdreader** 86.5%

## Supported Encryption

✅ **aescbc**: AES-CBC with PKCS#7 padding

❌ **Not yet supported**: aesgcm, secretbox, kms

## Architecture

- **cmd/etcd-secret-reader**: CLI entry point and output formatting
- **pkg/etcdreader**: etcd snapshot reading with MVCC decoding
- **pkg/decrypt**: AES-CBC decryption implementation

Uses official libraries: `go.etcd.io/bbolt`, `go.etcd.io/etcd/api/v3`, `k8s.io/api`

## Contributing

Contributions welcome! Please:
1. Write tests for new features
2. Run `make check` before submitting
3. Follow existing code style

## License

MIT
