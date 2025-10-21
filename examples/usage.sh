#!/bin/bash
# Example usage script for etcd-secret-reader

set -e

echo "=== etcd-secret-reader Usage Examples ==="
echo

# Build the tool
echo "Building etcd-secret-reader..."
go build -o etcd-secret-reader ./cmd/etcd-secret-reader
echo "Build complete!"
echo

# Configuration
SNAPSHOT="snapshot.db"
NAMESPACE="default"
SECRET_NAME="my-secret"
ENCRYPTION_KEY="y0xTt+U6xgRdNxe4nDYYsijOGgRDoUYC+wAwOKeNfPs="

# Example 1: List all secrets
echo "Example 1: List all secrets in the snapshot"
echo "Command:"
echo "  ./etcd-secret-reader --snapshot=$SNAPSHOT --list"
echo
# Uncomment to run:
# ./etcd-secret-reader --snapshot="$SNAPSHOT" --list
echo

# Example 2: Read a specific secret
echo "Example 2: Read and decrypt a specific secret"
echo "Command:"
echo "  ./etcd-secret-reader \\"
echo "    --snapshot=$SNAPSHOT \\"
echo "    --namespace=$NAMESPACE \\"
echo "    --name=$SECRET_NAME \\"
echo "    --key=\$ENCRYPTION_KEY"
echo
# Uncomment to run:
# ./etcd-secret-reader \
#   --snapshot="$SNAPSHOT" \
#   --namespace="$NAMESPACE" \
#   --name="$SECRET_NAME" \
#   --key="$ENCRYPTION_KEY"
echo

# Example 3: Read all secrets
echo "Example 3: Read and decrypt all secrets"
echo "Command:"
echo "  ./etcd-secret-reader \\"
echo "    --snapshot=$SNAPSHOT \\"
echo "    --key=\$ENCRYPTION_KEY"
echo
# Uncomment to run:
# ./etcd-secret-reader \
#   --snapshot="$SNAPSHOT" \
#   --key="$ENCRYPTION_KEY"
echo

# Example 4: Using environment variable for key
echo "Example 4: Using environment variable for encryption key"
echo "Command:"
echo "  export ETCD_ENCRYPTION_KEY='$ENCRYPTION_KEY'"
echo "  ./etcd-secret-reader \\"
echo "    --snapshot=$SNAPSHOT \\"
echo "    --namespace=$NAMESPACE \\"
echo "    --name=$SECRET_NAME \\"
echo "    --key=\$ETCD_ENCRYPTION_KEY"
echo
# Uncomment to run:
# export ETCD_ENCRYPTION_KEY="$ENCRYPTION_KEY"
# ./etcd-secret-reader \
#   --snapshot="$SNAPSHOT" \
#   --namespace="$NAMESPACE" \
#   --name="$SECRET_NAME" \
#   --key="$ETCD_ENCRYPTION_KEY"
echo

echo "=== Creating an etcd Snapshot ==="
echo
echo "If you need to create a snapshot from a running etcd cluster:"
echo
echo "ETCDCTL_API=3 etcdctl snapshot save snapshot.db \\"
echo "  --endpoints=https://127.0.0.1:2379 \\"
echo "  --cacert=/etc/kubernetes/pki/etcd/ca.crt \\"
echo "  --cert=/etc/kubernetes/pki/etcd/server.crt \\"
echo "  --key=/etc/kubernetes/pki/etcd/server.key"
echo

echo "=== Generating a New Encryption Key ==="
echo
echo "To generate a random 32-byte key for testing:"
echo "  head -c 32 /dev/urandom | base64"
echo
echo "Sample key: $(head -c 32 /dev/urandom | base64)"
echo
echo "Warning: Use the actual key from your cluster's EncryptionConfiguration"
echo "to decrypt existing secrets!"
