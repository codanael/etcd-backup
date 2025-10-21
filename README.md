# etcd-secret-reader

A command-line tool to read and decrypt Kubernetes secrets from encrypted etcd backups without requiring a full cluster restore.

## Features

- Read secrets from etcd snapshot files
- Decrypt secrets encrypted with AES-CBC encryption
- List all secrets in a snapshot
- Display decrypted secret data
- Uses official Kubernetes and etcd libraries

## Prerequisites

- etcd snapshot file (`.db` file)
- Encryption key used by your Kubernetes cluster (base64-encoded, 32 bytes)

## Installation

### Option 1: Download Pre-built Binary (Recommended)

Download the latest release for your platform from the [Releases page](../../releases):

**Linux/macOS:**
```bash
# Download and extract (replace VERSION and PLATFORM)
VERSION=v1.0.0
PLATFORM=linux-amd64  # or linux-arm64, darwin-amd64, darwin-arm64

wget https://github.com/codanael/etcd-backup/releases/download/${VERSION}/etcd-secret-reader-${VERSION}-${PLATFORM}.tar.gz
tar -xzf etcd-secret-reader-${VERSION}-${PLATFORM}.tar.gz
chmod +x etcd-secret-reader
sudo mv etcd-secret-reader /usr/local/bin/
```

**Windows:**
```powershell
# Download the .zip file and extract it
# Add the extracted directory to your PATH
```

### Option 2: Build from Source

**Using Make (recommended):**
```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Install to /usr/local/bin
make install
```

**Using Go directly:**
```bash
go build -o etcd-secret-reader ./cmd/etcd-secret-reader
```

### Verify Installation

```bash
etcd-secret-reader --version
```

## Usage

### List all secrets (without decryption)

```bash
./etcd-secret-reader --snapshot=/path/to/snapshot.db --list
```

### Read and decrypt a specific secret

```bash
./etcd-secret-reader \
  --snapshot=/path/to/snapshot.db \
  --namespace=default \
  --name=my-secret \
  --key=<base64-encoded-32-byte-key>
```

### Read and decrypt all secrets

```bash
./etcd-secret-reader \
  --snapshot=/path/to/snapshot.db \
  --key=<base64-encoded-32-byte-key>
```

## Command-line Flags

- `--snapshot` (required): Path to etcd snapshot file
- `--namespace`: Kubernetes namespace of the secret
- `--name`: Name of the secret
- `--key`: Base64-encoded 32-byte AES-CBC encryption key (required for decryption)
- `--key-name`: Name of the encryption key (default: "key1")
- `--list`: List all secrets without decrypting

## How It Works

### Kubernetes Encryption at Rest

Kubernetes can encrypt data before storing it in etcd using the `EncryptionConfiguration`:

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
              secret: <BASE64-ENCODED-32-BYTE-KEY>
      - identity: {}
```

### Encrypted Data Format

When secrets are encrypted with AES-CBC, they are stored in etcd with the following format:

```
k8s:enc:aescbc:v1:<keyName>:<IV><encrypted-data>
```

Where:
- `k8s:enc:aescbc:v1:` - Encryption prefix indicating AES-CBC provider
- `<keyName>` - Name of the encryption key used
- `<IV>` - Initialization vector (first 16 bytes)
- `<encrypted-data>` - AES-CBC encrypted payload

### Storage Path

Secrets are stored in etcd at the path:
```
/registry/secrets/<namespace>/<secret-name>
```

## Getting Your Encryption Key

### From EncryptionConfiguration File

If you have access to your cluster's EncryptionConfiguration file (usually at `/etc/kubernetes/encryption-config.yaml`):

```bash
# Extract the base64-encoded key
grep -A1 "secret:" /etc/kubernetes/encryption-config.yaml | tail -1 | awk '{print $2}'
```

### Generate a New Test Key

For testing purposes only:
```bash
head -c 32 /dev/urandom | base64
```

**Warning**: This will NOT decrypt existing secrets unless you use the actual key from your cluster.

## Creating an etcd Snapshot

If you need to create a snapshot from a running etcd cluster:

```bash
ETCDCTL_API=3 etcdctl snapshot save snapshot.db \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key
```

## Example

```bash
# 1. Create a test secret in Kubernetes
kubectl create secret generic test-secret \
  --from-literal=username=admin \
  --from-literal=password=secretpass

# 2. Take an etcd snapshot
ETCDCTL_API=3 etcdctl snapshot save snapshot.db

# 3. List secrets in the snapshot
./etcd-secret-reader --snapshot=snapshot.db --list

# 4. Decrypt the secret
./etcd-secret-reader \
  --snapshot=snapshot.db \
  --namespace=default \
  --name=test-secret \
  --key=y0xTt+U6xgRdNxe4nDYYsijOGgRDoUYC+wAwOKeNfPs=
```

Output:
```
Secret: default/test-secret
Type: Opaque
Data:
  username: admin
  password: secretpass
```

## Supported Encryption Providers

Currently supported:
- **aescbc**: AES-CBC with PKCS#7 padding (this tool)

Future support (not yet implemented):
- aesgcm: AES-GCM with random nonce
- secretbox: XSalsa20 and Poly1305
- kms: External Key Management Service

## Security Considerations

- **Protect your encryption keys**: Never commit keys to version control
- **Restrict access**: Limit access to snapshot files and encryption keys
- **Key rotation**: Kubernetes supports multiple keys for rotation
- **AES-CBC limitations**: Consider using aesgcm or secretbox for new deployments (more secure)

## Architecture

This tool leverages official Kubernetes and etcd libraries:

- `go.etcd.io/bbolt`: Read etcd snapshot files (BBolt database format)
- `k8s.io/apiserver/pkg/storage/value/encrypt/aes`: AES-CBC transformer from Kubernetes
- `k8s.io/api`: Kubernetes Secret types
- `k8s.io/apimachinery`: Kubernetes serialization

## License

MIT

## Contributing

Contributions welcome! Please open an issue or pull request.
