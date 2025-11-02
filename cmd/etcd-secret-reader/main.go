package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/codanael/etcd-secret-reader/pkg/decrypt"
	"github.com/codanael/etcd-secret-reader/pkg/etcdreader"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// version is set during build time via -ldflags
var version = "dev"

// isPrintable checks if a string contains only printable characters
func isPrintable(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// safePrintKey prints a key in a safe format for console output
func safePrintKey(key string) string {
	// If the key is printable, return it as-is
	if isPrintable(key) {
		return key
	}

	// If the key contains binary data, show it in quoted format
	// This will escape non-printable characters
	return fmt.Sprintf("%q (contains binary data)", key)
}

func main() {
	// Command line flags
	snapshotPath := flag.String("snapshot", "", "Path to etcd snapshot file (required)")
	namespace := flag.String("namespace", "", "Kubernetes namespace")
	secretName := flag.String("name", "", "Secret name")
	encryptionKey := flag.String("key", "", "Base64-encoded 32-byte AES-CBC encryption key (required)")
	keyName := flag.String("key-name", "key1", "Name of the encryption key")
	listOnly := flag.Bool("list", false, "List all secrets without decrypting")
	listAll := flag.Bool("list-all", false, "List all keys in the snapshot (for debugging)")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("etcd-secret-reader version %s\n", version)
		os.Exit(0)
	}

	if *snapshotPath == "" {
		fmt.Fprintf(os.Stderr, "Error: --snapshot is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Open etcd snapshot
	reader, err := etcdreader.NewReader(*snapshotPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening snapshot: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	// List all keys mode (for debugging)
	if *listAll {
		keys, err := reader.ListAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing all keys: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("All keys in snapshot (%d total):\n", len(keys))

		// Count how many are secrets
		secretCount := 0
		for _, k := range keys {
			if strings.HasPrefix(k, "/registry/secrets/") {
				secretCount++
			}
		}

		if secretCount > 0 {
			fmt.Printf("  (%d keys match /registry/secrets/ prefix)\n\n", secretCount)
		} else {
			fmt.Println("  (no keys match /registry/secrets/ prefix)")
			fmt.Println()
		}

		for _, k := range keys {
			safeKey := safePrintKey(k)
			// Highlight secrets
			if strings.HasPrefix(k, "/registry/secrets/") {
				fmt.Printf("  [SECRET] %s\n", safeKey)
			} else {
				fmt.Printf("  %s\n", safeKey)
			}
		}
		return
	}

	// List mode
	if *listOnly {
		secrets, err := reader.ListSecrets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing secrets: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Secrets in snapshot (%d found):\n", len(secrets))
		if len(secrets) == 0 {
			fmt.Println("  (no secrets found)")
			fmt.Println("\nTip: Use --list-all to see all keys in the snapshot and verify the correct prefix.")
		} else {
			for _, s := range secrets {
				fmt.Printf("  %s\n", s)
			}
		}
		return
	}

	// Decrypt mode - requires key
	if *encryptionKey == "" {
		fmt.Fprintf(os.Stderr, "Error: --key is required for decryption\n")
		flag.Usage()
		os.Exit(1)
	}

	// Decode encryption key
	keyBytes, err := base64.StdEncoding.DecodeString(*encryptionKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding encryption key: %v\n", err)
		os.Exit(1)
	}

	if len(keyBytes) != 32 {
		fmt.Fprintf(os.Stderr, "Error: encryption key must be 32 bytes (got %d bytes)\n", len(keyBytes))
		os.Exit(1)
	}

	// Create decryptor
	decryptor, err := decrypt.NewAESCBCDecryptor(keyBytes, *keyName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating decryptor: %v\n", err)
		os.Exit(1)
	}

	// Get specific secret or all secrets
	if *namespace != "" && *secretName != "" {
		// Try both standard Kubernetes and OpenShift secret paths
		var encryptedData []byte
		var err error

		keys := []string{
			fmt.Sprintf("/registry/secrets/%s/%s", *namespace, *secretName),
			fmt.Sprintf("/kubernetes.io/secrets/%s/%s", *namespace, *secretName),
		}

		for _, key := range keys {
			encryptedData, err = reader.Get(key)
			if err == nil {
				break
			}
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading secret: %v\n", err)
			os.Exit(1)
		}

		decryptedData, err := decryptor.Decrypt(encryptedData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error decrypting secret: %v\n", err)
			os.Exit(1)
		}

		// Parse and display secret
		if err := displaySecret(*namespace, *secretName, decryptedData); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing secret: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Get all secrets
		secrets, err := reader.ListSecrets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing secrets: %v\n", err)
			os.Exit(1)
		}

		for _, secretPath := range secrets {
			encryptedData, err := reader.Get(secretPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not read %s: %v\n", secretPath, err)
				continue
			}

			decryptedData, err := decryptor.Decrypt(encryptedData)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not decrypt %s: %v\n", secretPath, err)
				continue
			}

			// Parse path to get namespace and name
			// Path format: /registry/secrets/<namespace>/<name>
			ns, name := parseSecretPath(secretPath)
			if err := displaySecret(ns, name, decryptedData); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not parse %s: %v\n", secretPath, err)
			}
			fmt.Println()
		}
	}
}

func parseSecretPath(path string) (namespace, name string) {
	// Support both formats:
	// /registry/secrets/<namespace>/<name>
	// /kubernetes.io/secrets/<namespace>/<name>

	// Split path by '/' and filter out empty parts
	parts := strings.Split(path, "/")
	var nonEmptyParts []string
	for _, p := range parts {
		if p != "" {
			nonEmptyParts = append(nonEmptyParts, p)
		}
	}

	// Path should be: [prefix, "secrets", namespace, name]
	// e.g., ["registry", "secrets", "default", "my-secret"]
	// or ["kubernetes.io", "secrets", "openshift-etcd", "etcd-metric-signer"]
	if len(nonEmptyParts) >= 4 {
		namespace = nonEmptyParts[len(nonEmptyParts)-2]
		name = nonEmptyParts[len(nonEmptyParts)-1]
	}

	return
}

func displaySecret(namespace, name string, data []byte) error {
	fmt.Printf("Secret: %s/%s\n", namespace, name)

	// Try to detect format: protobuf vs JSON
	var secret *corev1.Secret
	var err error

	// Check if it's protobuf (starts with "k8s\x00")
	if len(data) > 4 && data[0] == 'k' && data[1] == '8' && data[2] == 's' && data[3] == 0 {
		// Decode protobuf
		secret, err = decodeProtobufSecret(data)
		if err != nil {
			return fmt.Errorf("failed to decode protobuf secret: %w", err)
		}
	} else {
		// Try JSON
		secret, err = decodeJSONSecret(data)
		if err != nil {
			return fmt.Errorf("failed to parse secret (tried both protobuf and JSON): %w", err)
		}
	}

	// Display type
	fmt.Printf("Type: %s\n", secret.Type)

	// Display data
	if len(secret.Data) > 0 {
		fmt.Println("Data:")
		for key, val := range secret.Data {
			fmt.Printf("  %s: %s\n", key, string(val))
		}
	}

	// Display string data if present
	if len(secret.StringData) > 0 {
		fmt.Println("StringData:")
		for key, val := range secret.StringData {
			fmt.Printf("  %s: %s\n", key, val)
		}
	}

	return nil
}

func decodeProtobufSecret(data []byte) (*corev1.Secret, error) {
	// Create a Kubernetes scheme and decoder
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core/v1 to scheme: %w", err)
	}

	// Create a codec factory
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDeserializer()

	// Decode the protobuf data
	obj, _, err := decoder.Decode(data, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil, fmt.Errorf("decoded object is not a Secret, got %T", obj)
	}

	return secret, nil
}

func decodeJSONSecret(data []byte) (*corev1.Secret, error) {
	var secret corev1.Secret
	if err := json.Unmarshal(data, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}
