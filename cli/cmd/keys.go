package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"filippo.io/age"
	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/pkg/natsauth"
)

// KeyExport is the plaintext format inside age-encrypted key export files.
type KeyExport struct {
	Version   int           `json:"version"`
	CreatedAt time.Time     `json:"created_at"`
	KeyCount  int           `json:"key_count"`
	Keys      []ExportedKey `json:"keys"`
}

// ExportedKey is one ENCRYPTION_KEYS entry in the export.
type ExportedKey struct {
	// KeyRef is the literal ENCRYPTION_KEYS key. New KMS-backed entries use
	// opaque refs such as "kek.Abc123" and "dek.Abc123"; legacy exports may
	// only have UserID.
	KeyRef string `json:"key_ref,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Key    []byte `json:"key"` // raw legacy key bytes or protobuf-encoded key record
}

var (
	keysConfigFile string
	keysOutput     string
	keysPassphrase string
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage encryption keys",
	Long:  "Commands for exporting and importing encryption keys, separate from data backups.",
}

var keysExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export encryption keys",
	Long: `Exports all Chatto encryption key records to a file, encrypted with a passphrase.

The export file is encrypted using age (age-encryption.org) and contains
all key-encryption keys and wrapped content-key records needed to decrypt
message bodies and encrypted durable user PII. Store this file securely —
anyone with the file and passphrase can decrypt encrypted Chatto content.

Use together with 'chatto backup' for complete disaster recovery:
  1. chatto backup -c chatto.toml
  2. chatto keys export -c chatto.toml -o keys.backup`,
	Run: runKeysExport,
}

var keysImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import encryption keys",
	Long: `Imports Chatto encryption key records from a file created by 'chatto keys export'.

By default, existing records are NOT overwritten. Records are only imported
when the ENCRYPTION_KEYS bucket does not already contain that key ref.

Use together with 'chatto restore' for complete disaster recovery:
  1. chatto restore backup.tar.gz -c chatto.toml
  2. chatto keys import keys.backup -c chatto.toml`,
	Args: cobra.ExactArgs(1),
	Run:  runKeysImport,
}

func init() {
	rootCmd.AddCommand(keysCmd)
	keysCmd.AddCommand(keysExportCmd)
	keysCmd.AddCommand(keysImportCmd)

	keysExportCmd.Flags().StringVarP(&keysConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	keysExportCmd.Flags().StringVarP(&keysOutput, "output", "o", "", "output file path (required)")
	keysExportCmd.Flags().StringVar(&keysPassphrase, "passphrase", "", "encryption passphrase (if not set, prompts interactively)")
	_ = keysExportCmd.MarkFlagRequired("output")

	keysImportCmd.Flags().StringVarP(&keysConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	keysImportCmd.Flags().StringVar(&keysPassphrase, "passphrase", "", "decryption passphrase (if not set, prompts interactively)")
}

func runKeysExport(cmd *cobra.Command, args []string) {
	cfg, err := config.ReadConfig(keysConfigFile)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}

	passphrase, err := getPassphrase(keysPassphrase, "Enter passphrase for key export: ", true)
	if err != nil {
		log.Fatal("Failed to read passphrase", "error", err)
	}

	nc, err := connectForKeys(cfg)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer nc.Close()

	ctx := context.Background()
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatal("Failed to create JetStream context", "error", err)
	}

	kv, err := js.KeyValue(ctx, "ENCRYPTION_KEYS")
	if err != nil {
		log.Fatal("Failed to open ENCRYPTION_KEYS bucket", "error", err)
	}

	keys, err := exportAllKeys(ctx, kv)
	if err != nil {
		log.Fatal("Failed to export keys", "error", err)
	}

	log.Info("Exported keys", "count", len(keys))

	if len(keys) == 0 {
		log.Warn("No encryption keys found. Nothing to export.")
		return
	}

	if err := encryptKeysToFile(keys, passphrase, keysOutput); err != nil {
		log.Fatal("Failed to write encrypted key export", "error", err)
	}

	log.Info("Key export complete", "file", keysOutput, "keys", len(keys))
	log.Warn("This file contains your encryption keys. Store it securely!")
}

func runKeysImport(cmd *cobra.Command, args []string) {
	importFile := args[0]

	cfg, err := config.ReadConfig(keysConfigFile)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}

	passphrase, err := getPassphrase(keysPassphrase, "Enter passphrase for key import: ", false)
	if err != nil {
		log.Fatal("Failed to read passphrase", "error", err)
	}

	keys, err := decryptKeysFromFile(importFile, passphrase)
	if err != nil {
		log.Fatal("Failed to decrypt keys (wrong passphrase?)", "error", err)
	}

	log.Info("Decrypted keys from export", "count", len(keys))

	nc, err := connectForKeys(cfg)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer nc.Close()

	ctx := context.Background()
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatal("Failed to create JetStream context", "error", err)
	}

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "ENCRYPTION_KEYS",
		Description: "User encryption keys (excluded from backups)",
		Storage:     jetstream.FileStorage,
	})
	if err != nil {
		log.Fatal("Failed to open ENCRYPTION_KEYS bucket", "error", err)
	}

	imported, skipped, err := importKeys(ctx, kv, keys)
	if err != nil {
		log.Fatal("Failed to import keys", "error", err)
	}

	log.Info("Key import complete", "imported", imported, "skipped_existing", skipped)
}

func keyRefForImport(key ExportedKey) string {
	if key.KeyRef != "" {
		return key.KeyRef
	}
	if key.UserID == "" {
		return ""
	}
	return "user." + key.UserID
}

func validateEncryptionKeyRecord(keyRef string, data []byte) error {
	switch {
	case strings.HasPrefix(keyRef, "dek."):
		return dekstore.ValidateUserDataEncryptionKeyRecord(keyRef, data)
	case kms.IsKeyRef(keyRef):
		return kms.ValidateUserKeyEncryptionKeyRecord(keyRef, data)
	default:
		return fmt.Errorf("unknown ENCRYPTION_KEYS key ref prefix: %s", keyRef)
	}
}

func validateExportedKey(key ExportedKey) (string, error) {
	keyRef := keyRefForImport(key)
	if keyRef == "" {
		return "", fmt.Errorf("missing key_ref/user_id")
	}
	if err := validateEncryptionKeyRecord(keyRef, key.Key); err != nil {
		return "", err
	}
	return keyRef, nil
}

func importKeys(ctx context.Context, kv jetstream.KeyValue, keys []ExportedKey) (imported int, skipped int, err error) {
	keyRefs := make([]string, len(keys))
	for i, key := range keys {
		keyRef, err := validateExportedKey(key)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid exported key %q: %w", keyRefForImport(key), err)
		}
		keyRefs[i] = keyRef
	}

	for i, key := range keys {
		keyRef := keyRefs[i]
		_, err := kv.Create(ctx, keyRef, key.Key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				skipped++
				continue
			}
			return imported, skipped, fmt.Errorf("failed to import key %s: %w", keyRef, err)
		}
		imported++
	}
	return imported, skipped, nil
}

// exportAllKeys reads all entries from the ENCRYPTION_KEYS KV bucket.
func exportAllKeys(ctx context.Context, kv jetstream.KeyValue) ([]ExportedKey, error) {
	keys, err := kv.Keys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	var exported []ExportedKey
	for _, key := range keys {
		entry, err := kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get key %s: %w", key, err)
		}
		if err := validateEncryptionKeyRecord(key, entry.Value()); err != nil {
			return nil, fmt.Errorf("invalid ENCRYPTION_KEYS record %s: %w", key, err)
		}

		exportedKey := ExportedKey{
			KeyRef: key,
			Key:    entry.Value(),
		}
		if strings.HasPrefix(key, "user.") {
			exportedKey.UserID = strings.TrimPrefix(key, "user.")
		}
		exported = append(exported, exportedKey)
	}

	return exported, nil
}

// encryptKeysToFile encrypts keys with age and writes them to a file.
func encryptKeysToFile(keys []ExportedKey, passphrase, filePath string) error {
	export := KeyExport{
		Version:   3,
		CreatedAt: time.Now().UTC(),
		KeyCount:  len(keys),
		Keys:      keys,
	}

	plaintext, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	outFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("failed to create age recipient: %w", err)
	}

	w, err := age.Encrypt(outFile, recipient)
	if err != nil {
		return fmt.Errorf("failed to initialize encryption: %w", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return fmt.Errorf("failed to write encrypted data: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to finalize encryption: %w", err)
	}

	return nil
}

// decryptKeysFromFile reads an age-encrypted key export and decrypts it.
func decryptKeysFromFile(filePath, passphrase string) ([]ExportedKey, error) {
	inFile, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer inFile.Close()

	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed to create age identity: %w", err)
	}

	r, err := age.Decrypt(inFile, identity)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read decrypted data: %w", err)
	}

	var export KeyExport
	if err := json.Unmarshal(plaintext, &export); err != nil {
		return nil, fmt.Errorf("failed to parse key export: %w", err)
	}

	if export.Version != 2 && export.Version != 3 {
		return nil, fmt.Errorf("unsupported key export version: %d (expected 2 or 3)", export.Version)
	}

	return export.Keys, nil
}

// getPassphrase reads a passphrase from the flag value, stdin pipe, or interactive prompt.
// If confirm is true, prompts for confirmation (export use case) — only in interactive mode.
func getPassphrase(flagValue string, prompt string, confirm bool) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}

	// If stdin is piped, read a single line from it (no confirmation possible).
	if !term.IsTerminal(int(syscall.Stdin)) {
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("failed to read passphrase from stdin: %w", err)
			}
			return "", fmt.Errorf("passphrase cannot be empty")
		}
		pass := strings.TrimRight(scanner.Text(), "\r\n")
		if pass == "" {
			return "", fmt.Errorf("passphrase cannot be empty")
		}
		return pass, nil
	}

	// Interactive: prompt with hidden input.
	fmt.Fprint(os.Stderr, prompt)
	pass, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr) // newline after hidden input
	if err != nil {
		return "", err
	}

	if len(pass) == 0 {
		return "", fmt.Errorf("passphrase cannot be empty")
	}

	if confirm {
		fmt.Fprint(os.Stderr, "Confirm passphrase: ")
		pass2, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if string(pass) != string(pass2) {
			return "", fmt.Errorf("passphrases do not match")
		}
	}

	return string(pass), nil
}

// connectForKeys connects to NATS for key operations (same pattern as backup).
func connectForKeys(cfg config.ChattoConfig) (*nats.Conn, error) {
	authOpts, err := natsauth.ConnectOptions(cfg.NATS.Client.NATSAuthConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to get NATS auth options: %w", err)
	}
	nc, err := nats.Connect(cfg.NATS.Client.URL, authOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	return nc, nil
}
