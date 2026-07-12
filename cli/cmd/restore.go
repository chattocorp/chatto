package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/charmbracelet/log"
	"github.com/nats-io/jsm.go"
	"github.com/nats-io/jsm.go/api"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/embedded_nats"
	"hmans.de/chatto/pkg/natsauth"
)

var (
	restoreConfigFile string
	restoreConflict   string
	restorePassphrase string
)

var restoreCmd = &cobra.Command{
	Use:   "restore <archive>",
	Short: "Restore a backup archive",
	Long: `Restores a Chatto backup created by 'chatto backup'.

For embedded NATS: starts a temporary NATS server, restores all streams,
then shuts down. Make sure Chatto is not running (the data directory
must not be locked by another process).

For external NATS: connects to the running NATS server via the client
config. Make sure the Chatto application is stopped to avoid concurrent
writes during restore.

Conflict handling:
  --conflict=error     Fail if any stream already exists (default)
  --conflict=skip      Skip streams that already exist
  --conflict=overwrite Delete and recreate existing streams`,
	Args: cobra.ExactArgs(1),
	RunE: runRestore,
}

func init() {
	rootCmd.AddCommand(restoreCmd)
	restoreCmd.Flags().StringVarP(&restoreConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	restoreCmd.Flags().StringVar(&restoreConflict, "conflict", "error", "conflict handling: error, skip, overwrite")
	restoreCmd.Flags().StringVar(&restorePassphrase, "passphrase", "", "decryption passphrase for encrypted backups (if not set, prompts interactively)")
}

func runRestore(cmd *cobra.Command, args []string) error {
	archivePath := args[0]
	startTime := time.Now()
	archive, err := openRestoreArchive(archivePath)
	if err != nil {
		return fmt.Errorf("invalid restore archive: %w", err)
	}
	defer archive.Close()

	// Validate conflict flag
	switch restoreConflict {
	case "error", "skip", "overwrite":
		// valid
	default:
		return fmt.Errorf("invalid --conflict value %q, must be: error, skip, overwrite", restoreConflict)
	}

	cfg, err := config.ReadConfig(restoreConfigFile)
	if err != nil {
		return fmt.Errorf("failed to read configuration: %w", err)
	}

	log.Info("Starting restore", "archive", archivePath, "conflict", restoreConflict)

	// Extract archive to temp directory
	tempDir, err := os.MkdirTemp("", "chatto-restore-")
	if err != nil {
		log.Fatal("Failed to create temp directory", "error", err)
	}
	defer os.RemoveAll(tempDir)

	// Check if the archive is age-encrypted
	encrypted, err := isAgeEncryptedReader(archive)
	if err != nil {
		return fmt.Errorf("failed to check archive encryption: %w", err)
	}

	if encrypted {
		log.Info("Archive is encrypted, decryption required")
		passphrase, err := getPassphrase(restorePassphrase, "Enter passphrase for backup decryption: ", false)
		if err != nil {
			return fmt.Errorf("failed to read passphrase: %w", err)
		}

		log.Info("Decrypting and extracting archive...")
		if err := extractEncryptedTarGzReader(archive, tempDir, passphrase); err != nil {
			return fmt.Errorf("failed to decrypt/extract archive: %w", err)
		}
	} else {
		log.Info("Extracting archive...")
		if err := readTarGz(io.LimitReader(archive, maxRestoreArchiveCompressedBytes+1), tempDir); err != nil {
			return fmt.Errorf("failed to extract archive: %w", err)
		}
	}

	// Find the backup directory inside the temp dir (archive contains a timestamp-named dir)
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("failed to read temp directory: %w", err)
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return fmt.Errorf("unexpected archive structure: expected a single directory")
	}
	backupDir := filepath.Join(tempDir, entries[0].Name())

	// Read manifest
	manifestPath := filepath.Join(backupDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest BackupManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}
	if err := validateBackupManifest(manifest); err != nil {
		return fmt.Errorf("invalid backup manifest: %w", err)
	}

	log.Info("Backup info",
		"created_at", manifest.CreatedAt,
		"streams", manifest.Stats.TotalStreams,
		"size", formatBytes(manifest.Stats.TotalBytes),
	)

	// Connect to NATS
	nc, embeddedServer, err := connectForRestore(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()
	if embeddedServer != nil {
		defer func() {
			embeddedServer.Shutdown()
			embeddedServer.WaitForShutdown()
			log.Info("Temporary NATS server shut down")
		}()
	}

	ctx := context.Background()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	mgr, err := jsm.New(nc)
	if err != nil {
		return fmt.Errorf("failed to create JSM manager: %w", err)
	}

	// Build set of existing streams for conflict detection.
	existingStreams := make(map[string]bool)
	streamLister := js.ListStreams(ctx)
	for info := range streamLister.Info() {
		existingStreams[info.Config.Name] = true
	}
	if err := streamLister.Err(); err != nil {
		return fmt.Errorf("failed to enumerate existing streams: %w", err)
	}

	// Restore each stream from the manifest
	streamsDir := filepath.Join(backupDir, "streams")
	targetReplicas := cfg.NATS.ReplicasOrDefault()
	var restored, skipped, failed int

	for i, streamInfo := range manifest.Streams {
		prefix := fmt.Sprintf("[%d/%d]", i+1, len(manifest.Streams))

		// Skip entries that were skipped during backup
		if streamInfo.Type == "skipped" || streamInfo.Error != "" {
			log.Info(fmt.Sprintf("%s Skipping %s (not in backup)", prefix, streamInfo.Name))
			skipped++
			continue
		}

		// Check if stream directory exists in backup
		streamDir := filepath.Join(streamsDir, streamInfo.Name)
		if _, err := os.Stat(streamDir); os.IsNotExist(err) {
			log.Warn(fmt.Sprintf("%s Stream directory missing in backup, skipping", prefix), "name", streamInfo.Name)
			skipped++
			continue
		}

		// Handle conflicts with existing streams
		if existingStreams[streamInfo.Name] {
			switch restoreConflict {
			case "error":
				return fmt.Errorf("stream %q already exists; use --conflict=skip or --conflict=overwrite", streamInfo.Name)
			case "skip":
				log.Info(fmt.Sprintf("%s Skipping %s (already exists)", prefix, streamInfo.Name))
				skipped++
				continue
			case "overwrite":
				log.Info(fmt.Sprintf("%s Deleting existing stream %s", prefix, streamInfo.Name))
				if err := js.DeleteStream(ctx, streamInfo.Name); err != nil {
					log.Error("Failed to delete existing stream", "name", streamInfo.Name, "error", err)
					failed++
					continue
				}
			}
		}

		log.Info(fmt.Sprintf("%s Restoring %s (%s, %d messages)", prefix, streamInfo.Name, streamInfo.Type, streamInfo.Messages))

		var restoreOpts []jsm.SnapshotOption
		override, err := restoreConfigForTarget(streamDir, streamInfo.Name, targetReplicas)
		if err != nil {
			log.Error("Failed to read restore metadata", "name", streamInfo.Name, "error", err)
			failed++
			continue
		}
		if override != nil {
			log.Info("Adjusting stream replicas for restore target",
				"name", streamInfo.Name,
				"backup_replicas", override.backupReplicas,
				"restore_replicas", override.config.Replicas,
			)
			restoreOpts = append(restoreOpts, jsm.RestoreConfiguration(override.config))
		}

		_, _, err = mgr.RestoreSnapshotFromDirectory(ctx, streamInfo.Name, streamDir, restoreOpts...)
		if err != nil {
			log.Error("Failed to restore stream", "name", streamInfo.Name, "error", err)
			failed++
			continue
		}

		log.Info(fmt.Sprintf("%s  Done", prefix))
		restored++
	}

	duration := time.Since(startTime)
	log.Info("Restore complete",
		"restored", restored,
		"skipped", skipped,
		"failed", failed,
		"duration", duration.Round(time.Millisecond),
	)

	if failed > 0 {
		log.Warn("Some streams failed to restore. Check the errors above.")
	}

	if manifestIncludesEncryptionKeys(manifest) {
		log.Info("Encryption keys were included in the backup and have been restored. " +
			"Encrypted message bodies should be readable by the application.")
	} else {
		log.Warn("Encryption keys were NOT included in this backup. " +
			"Encrypted message bodies cannot be decrypted without them — restore your " +
			"encryption keys separately, or recreate the backup with --include-keys.")
	}
	return nil
}

type restoreConfigOverride struct {
	config         api.StreamConfig
	backupReplicas int
}

func restoreConfigForTarget(streamDir, streamName string, targetReplicas int) (*restoreConfigOverride, error) {
	if targetReplicas <= 0 {
		targetReplicas = 1
	}

	metaPath := filepath.Join(streamDir, "backup.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	var req api.JSApiStreamRestoreRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	if req.Config.Name != streamName {
		return nil, fmt.Errorf("snapshot metadata stream name %q does not match manifest name %q", req.Config.Name, streamName)
	}

	if req.Config.Replicas == targetReplicas {
		return nil, nil
	}

	backupReplicas := req.Config.Replicas
	req.Config.Replicas = targetReplicas
	return &restoreConfigOverride{
		config:         req.Config,
		backupReplicas: backupReplicas,
	}, nil
}

// manifestIncludesEncryptionKeys reports whether KV_ENCRYPTION_KEYS was actually
// backed up (i.e. present and not marked skipped/failed in the manifest).
func manifestIncludesEncryptionKeys(m BackupManifest) bool {
	for _, s := range m.Streams {
		if s.Name != "KV_ENCRYPTION_KEYS" {
			continue
		}
		return s.Type != "skipped" && s.Error == ""
	}
	return false
}

func validateBackupManifest(manifest BackupManifest) error {
	seen := make(map[string]struct{}, len(manifest.Streams))
	for _, stream := range manifest.Streams {
		name := stream.Name
		if name == "" || filepath.IsAbs(name) || name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsAny(name, "/\\.*> \t\r\n") {
			return fmt.Errorf("invalid stream name %q", name)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate stream name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func openRestoreArchive(path string) (*os.File, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("restore archive must be a regular file, not a symlink or special file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fileInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !fileInfo.Mode().IsRegular() || !os.SameFile(pathInfo, fileInfo) {
		file.Close()
		return nil, fmt.Errorf("restore archive changed while it was being opened")
	}
	if fileInfo.Size() > maxRestoreArchiveCompressedBytes {
		file.Close()
		return nil, fmt.Errorf("restore archive exceeds the compressed-size limit of %d bytes", maxRestoreArchiveCompressedBytes)
	}
	return file, nil
}

// connectForRestore establishes a NATS connection for restore operations.
// For embedded NATS: starts a temporary server with no TCP listener.
// For external NATS: connects via the client config.
func connectForRestore(cfg config.ChattoConfig) (*nats.Conn, *server.Server, error) {
	if cfg.NATS.Embedded.Enabled {
		log.Info("Starting temporary NATS server for restore", "data_dir", cfg.NATS.Embedded.DataDir)

		// Start embedded NATS with no listeners (in-process only)
		opts := &server.Options{
			JetStream:  true,
			StoreDir:   cfg.NATS.Embedded.DataDir,
			NoSigs:     true,
			DontListen: true,
		}

		ns, err := server.NewServer(opts)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create NATS server: %w", err)
		}

		ns.Start()
		if !ns.ReadyForConnections(4 * time.Second) {
			return nil, nil, fmt.Errorf("NATS server failed to start")
		}

		log.Info("Temporary NATS server ready")

		nc, err := nats.Connect(nats.DefaultURL, embedded_nats.InProcessConnectOption(ns))
		if err != nil {
			ns.Shutdown()
			return nil, nil, fmt.Errorf("failed to connect to embedded NATS: %w", err)
		}

		return nc, ns, nil
	}

	// External NATS: connect via client config
	authOpts, err := natsauth.ConnectOptions(cfg.NATS.Client.NATSAuthConfig())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get NATS auth options: %w", err)
	}

	nc, err := nats.Connect(cfg.NATS.Client.URL, authOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	log.Info("Connected to NATS", "url", nc.ConnectedUrl())
	return nc, nil, nil
}

// isAgeEncrypted checks if a file starts with the age encryption header.
func isAgeEncrypted(filePath string) (bool, error) {
	f, err := openRestoreArchive(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	return isAgeEncryptedReader(f)
}

func isAgeEncryptedReader(r io.ReadSeeker) (bool, error) {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	header := make([]byte, 30)
	n, err := r.Read(header)
	if _, seekErr := r.Seek(0, io.SeekStart); seekErr != nil {
		return false, seekErr
	}
	if n == 0 || err != nil {
		return false, nil
	}

	return strings.HasPrefix(string(header[:n]), "age-encryption.org/v1\n"), nil
}

// extractEncryptedTarGz decrypts an age-encrypted .tar.gz archive and extracts it.
func extractEncryptedTarGz(archiveFile, destDir, passphrase string) error {
	inFile, err := openRestoreArchive(archiveFile)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer inFile.Close()
	return extractEncryptedTarGzReader(inFile, destDir, passphrase)
}

func extractEncryptedTarGzReader(r io.Reader, destDir, passphrase string) error {
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return fmt.Errorf("failed to create age identity: %w", err)
	}

	ageReader, err := age.Decrypt(io.LimitReader(r, maxRestoreArchiveCompressedBytes+1), identity)
	if err != nil {
		return fmt.Errorf("decryption failed (wrong passphrase?): %w", err)
	}

	return readTarGz(ageReader, destDir)
}
