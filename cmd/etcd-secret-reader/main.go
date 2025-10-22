package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/codanael/etcd-secret-reader/pkg/decrypt"
	"github.com/codanael/etcd-secret-reader/pkg/etcdreader"
)

// version is set during build time via -ldflags
var version = "dev"

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
		for _, k := range keys {
			fmt.Printf("  %s\n", k)
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
		// Get specific secret
		key := fmt.Sprintf("/registry/secrets/%s/%s", *namespace, *secretName)
		encryptedData, err := reader.Get(key)
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
	// Path format: /registry/secrets/<namespace>/<name>
	parts := []rune(path)
	slashCount := 0
	nsStart := -1
	nsEnd := -1
	nameStart := -1

	for i, c := range parts {
		if c == '/' {
			slashCount++
			if slashCount == 3 {
				nsStart = i + 1
			} else if slashCount == 4 {
				nsEnd = i
				nameStart = i + 1
			}
		}
	}

	if nsStart > 0 && nsEnd > nsStart {
		namespace = string(parts[nsStart:nsEnd])
	}
	if nameStart > 0 {
		name = string(parts[nameStart:])
	}

	return
}

func displaySecret(namespace, name string, data []byte) error {
	// Parse Kubernetes Secret
	var secret map[string]interface{}
	if err := json.Unmarshal(data, &secret); err != nil {
		return fmt.Errorf("failed to parse secret JSON: %w", err)
	}

	fmt.Printf("Secret: %s/%s\n", namespace, name)

	// Display type
	if secretType, ok := secret["type"].(string); ok {
		fmt.Printf("Type: %s\n", secretType)
	}

	// Display data
	if secretData, ok := secret["data"].(map[string]interface{}); ok {
		fmt.Println("Data:")
		for key, val := range secretData {
			if valStr, ok := val.(string); ok {
				// Data is base64-encoded in Kubernetes secrets
				decoded, err := base64.StdEncoding.DecodeString(valStr)
				if err == nil {
					fmt.Printf("  %s: %s\n", key, string(decoded))
				} else {
					fmt.Printf("  %s: %s (base64)\n", key, valStr)
				}
			}
		}
	}

	return nil
}
