package cmd

import (
	"archive/tar"
	"compress/gzip"
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
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsauth"
)

// BackupManifest contains metadata about a backup
type BackupManifest struct {
	Version   int                `json:"version"`
	CreatedAt time.Time          `json:"created_at"`
	Streams   []StreamBackupInfo `json:"streams"`
	Stats     BackupStats        `json:"stats"`
}

// StreamBackupInfo contains information about a backed up stream
type StreamBackupInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "stream", "kv", "object_store"
	Messages uint64 `json:"messages"`
	Bytes    uint64 `json:"bytes"`
	Error    string `json:"error,omitempty"`
}

// BackupStats contains aggregate statistics about the backup
type BackupStats struct {
	TotalStreams int    `json:"total_streams"`
	TotalBytes   uint64 `json:"total_bytes"`
	DurationMs   int64  `json:"duration_ms"`
	Skipped      int    `json:"skipped"`
	Failed       int    `json:"failed"`
}

var (
	backupConfigFile  string
	backupOutput      string
	backupEncrypt     bool
	backupPassphrase  string
	backupIncludeKeys bool
)

const (
	maxRestoreArchiveCompressedBytes = int64(128 * 1024 * 1024 * 1024)
	maxRestoreArchiveEntries         = 100_000
	maxRestoreArchiveFileBytes       = int64(64 * 1024 * 1024 * 1024)
	maxRestoreArchiveExpandedBytes   = int64(1024 * 1024 * 1024 * 1024)
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a backup of all Chatto data",
	Long: `Creates a complete backup of all NATS JetStream data including:
- Instance-level KV buckets (users, spaces, memberships)
- Instance event streams (audit trail)
- Instance assets (avatars, icons)
- Per-space KV buckets (rooms, memberships)
- Per-space event streams (messages)
- Per-space bodies (message bodies)
- Per-space reactions
- Per-space assets (attachments)

Excluded from backups by default:
- Encryption keys (security: keeps backup data encrypted at rest)
- User presence (ephemeral, memory-only)
- Retired standalone link preview cache, if present (regeneratable)
- Asset cache (regeneratable)

Pass --include-keys to include KV_ENCRYPTION_KEYS in the archive. This
makes the backup self-contained (encrypted message bodies become
restorable) but means anyone with the archive can decrypt the contents,
so the archive itself must be treated as sensitive (e.g. only stored on
trusted media or used in combination with --encrypt).

The backup is saved as a .tar.gz archive. Use --encrypt to protect
the archive with a passphrase (uses age encryption).`,
	Run: runBackup,
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.Flags().StringVarP(&backupConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "output path for the backup archive (default: backups/<timestamp>.tar.gz)")
	backupCmd.Flags().BoolVar(&backupEncrypt, "encrypt", false, "encrypt the backup with a passphrase (age encryption)")
	backupCmd.Flags().StringVar(&backupPassphrase, "passphrase", "", "encryption passphrase (if not set, prompts interactively)")
	backupCmd.Flags().BoolVar(&backupIncludeKeys, "include-keys", false, "include KV_ENCRYPTION_KEYS in the archive (treat the archive as sensitive)")
}

func runBackup(cmd *cobra.Command, args []string) {
	startTime := time.Now()

	cfg, err := config.ReadConfig(backupConfigFile)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}

	// Get passphrase early if encrypting
	var passphrase string
	if backupEncrypt {
		var err error
		passphrase, err = getPassphrase(backupPassphrase, "Enter passphrase for backup encryption: ", true)
		if err != nil {
			log.Fatal("Failed to read passphrase", "error", err)
		}
	}

	log.Info("Starting backup...")

	// Get NATS auth options and connect
	authOpts, err := natsauth.ConnectOptions(cfg.NATS.Client.NATSAuthConfig())
	if err != nil {
		log.Fatal("Failed to get NATS auth options", "error", err)
	}

	nc, err := nats.Connect(cfg.NATS.Client.URL, authOpts...)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer nc.Close()

	ctx := context.Background()

	// Create JetStream context for enumeration
	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatal("Failed to create JetStream context", "error", err)
	}

	// Create jsm.go manager for snapshots
	mgr, err := jsm.New(nc)
	if err != nil {
		log.Fatal("Failed to create JSM manager", "error", err)
	}

	// Determine output path
	ext := ".tar.gz"
	if backupEncrypt {
		ext = ".tar.gz.age"
	}

	var archivePath string
	if backupOutput != "" {
		archivePath = backupOutput
	} else {
		timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
		archivePath = filepath.Join("backups", timestamp+ext)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(archivePath), 0755); err != nil {
		log.Fatal("Failed to create output directory", "error", err)
	}

	manifest, err := createBackupArchive(ctx, js, mgr, archivePath, passphrase, backupEncrypt, backupIncludeKeys, startTime)
	if err != nil {
		log.Fatal("Failed to create backup archive", "error", err)
	} else {
		log.Info("Backup complete",
			"streams", manifest.Stats.TotalStreams,
			"skipped", manifest.Stats.Skipped,
			"size", formatBytes(manifest.Stats.TotalBytes),
			"duration", time.Duration(manifest.Stats.DurationMs)*time.Millisecond,
		)
		log.Info("Archive created", "path", archivePath)
		if backupIncludeKeys {
			log.Warn("Encryption keys are included in this backup. " +
				"Anyone with this archive can decrypt all encrypted content. " +
				"Treat the archive as sensitive material — keep it on trusted media or use --encrypt.")
		} else {
			log.Warn("Encryption keys are excluded from backups by default. " +
				"Encrypted message bodies and durable user PII in this backup cannot be decrypted without the keys. " +
				"Back up your encryption keys separately, or pass --include-keys to embed them.")
		}
	}
}

func createBackupArchive(ctx context.Context, js jetstream.JetStream, mgr *jsm.Manager, archivePath, passphrase string, encrypt, includeKeys bool, startTime time.Time) (BackupManifest, error) {
	staging, err := newBackupStaging(archivePath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer staging.cleanup()
	backupDir, streamsDir := staging.backupDir, staging.streamsDir

	streamNames, err := enumerateStreams(ctx, js)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("failed to enumerate streams: %w", err)
	}
	log.Info("Found streams to backup", "count", len(streamNames))

	manifest := BackupManifest{Version: 1, CreatedAt: startTime.UTC(), Streams: make([]StreamBackupInfo, 0, len(streamNames))}
	for i, streamName := range streamNames {
		info := backupStream(ctx, mgr, streamName, streamsDir, i+1, len(streamNames), includeKeys)
		manifest.Streams = append(manifest.Streams, info)
		switch {
		case info.Error != "":
			manifest.Stats.Failed++
		case info.Type == "skipped":
			manifest.Stats.Skipped++
		default:
			manifest.Stats.TotalBytes += info.Bytes
		}
	}
	manifest.Stats.TotalStreams = len(streamNames) - manifest.Stats.Skipped - manifest.Stats.Failed
	manifest.Stats.DurationMs = time.Since(startTime).Milliseconds()

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return BackupManifest{}, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "manifest.json"), manifestData, 0600); err != nil {
		return BackupManifest{}, fmt.Errorf("failed to write manifest: %w", err)
	}
	if err := secureBackupStaging(backupDir); err != nil {
		return BackupManifest{}, err
	}

	if encrypt {
		err = createEncryptedTarGz(backupDir, archivePath, passphrase)
	} else {
		err = createTarGz(backupDir, archivePath)
	}
	if err != nil {
		return BackupManifest{}, err
	}
	return manifest, nil
}

type backupStaging struct {
	root       string
	backupDir  string
	streamsDir string
}

func newBackupStaging(archivePath string) (*backupStaging, error) {
	root, err := os.MkdirTemp(filepath.Dir(archivePath), ".chatto-backup-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create private backup staging directory: %w", err)
	}
	staging := &backupStaging{root: root, backupDir: filepath.Join(root, "backup")}
	staging.streamsDir = filepath.Join(staging.backupDir, "streams")
	if err := os.MkdirAll(staging.streamsDir, 0700); err != nil {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("failed to create backup staging layout: %w", err)
	}
	return staging, nil
}

func (s *backupStaging) cleanup() {
	if s != nil {
		_ = os.RemoveAll(s.root)
	}
}

func secureBackupStaging(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			if err := os.Chmod(path, 0700); err != nil {
				return fmt.Errorf("failed to secure backup directory %s: %w", path, err)
			}
		case info.Mode().IsRegular():
			if err := os.Chmod(path, 0600); err != nil {
				return fmt.Errorf("failed to secure backup file %s: %w", path, err)
			}
		default:
			return fmt.Errorf("unsupported file in backup staging: %s", path)
		}
		return nil
	})
}

// enumerateStreams returns all stream names from JetStream
func enumerateStreams(ctx context.Context, js jetstream.JetStream) ([]string, error) {
	var names []string

	streamLister := js.ListStreams(ctx)
	for info := range streamLister.Info() {
		names = append(names, info.Config.Name)
	}

	if err := streamLister.Err(); err != nil {
		return nil, err
	}

	return names, nil
}

// backupStream backs up a single stream and returns info about the backup
func backupStream(ctx context.Context, mgr *jsm.Manager, streamName, streamsDir string, current, total int, includeKeys bool) StreamBackupInfo {
	streamType := classifyStream(streamName)
	prefix := fmt.Sprintf("[%d/%d]", current, total)

	// Check explicit skip list
	if reason := skipReason(streamName, includeKeys); reason != "" {
		log.Info(fmt.Sprintf("%s Skipping %s: %s", prefix, streamName, reason))
		return StreamBackupInfo{
			Name: streamName,
			Type: "skipped",
		}
	}

	stream, err := mgr.LoadStream(streamName)
	if err != nil {
		log.Error("Failed to load stream", "name", streamName, "error", err)
		return StreamBackupInfo{
			Name:  streamName,
			Type:  streamType,
			Error: err.Error(),
		}
	}

	// Skip memory-storage streams not in the explicit list
	if stream.Storage() == api.MemoryStorage {
		log.Info(fmt.Sprintf("%s Skipping %s: memory storage", prefix, streamName))
		return StreamBackupInfo{
			Name: streamName,
			Type: "skipped",
		}
	}

	// Get stream state for message count
	state, err := stream.State()
	if err != nil {
		log.Error("Failed to get stream state", "name", streamName, "error", err)
		return StreamBackupInfo{
			Name:  streamName,
			Type:  streamType,
			Error: err.Error(),
		}
	}

	log.Info(fmt.Sprintf("%s Backing up %s (%s, %d messages)", prefix, streamName, streamType, state.Msgs))

	streamDir := filepath.Join(streamsDir, streamName)

	var bytesReceived uint64

	_, err = stream.SnapshotToDirectory(ctx, streamDir,
		jsm.SnapshotConsumers(),
		jsm.SnapshotNotify(func(p jsm.SnapshotProgress) {
			bytesReceived = p.BytesReceived()
		}),
	)
	if err != nil {
		log.Error("Failed to snapshot stream", "name", streamName, "error", err)
		return StreamBackupInfo{
			Name:  streamName,
			Type:  streamType,
			Error: err.Error(),
		}
	}

	log.Info(fmt.Sprintf("%s  Done: %s", prefix, formatBytes(bytesReceived)))

	return StreamBackupInfo{
		Name:     streamName,
		Type:     streamType,
		Messages: state.Msgs,
		Bytes:    bytesReceived,
	}
}

// skipReason returns a human-readable reason if the stream should be skipped,
// or an empty string if it should be backed up. When includeKeys is true,
// KV_ENCRYPTION_KEYS is backed up; the archive must then be treated as sensitive.
func skipReason(name string, includeKeys bool) string {
	switch name {
	case "KV_MEMORY_CACHE":
		return "ephemeral (memory storage)"
	case "KV_USER_PRESENCE":
		return "ephemeral (memory storage)"
	case "KV_CALL_STATE":
		return "ephemeral (memory storage)"
	case "KV_ENCRYPTION_KEYS":
		if includeKeys {
			return ""
		}
		return "security (keys excluded from backups; pass --include-keys to override)"
	case "KV_LINK_PREVIEW_CACHE":
		return "cache (regeneratable)"
	case "KV_AUTH_TOKENS":
		return "security (prevents token leakage)"
	}
	if strings.HasPrefix(name, "OBJ_ASSET_CACHE") {
		return "cache (regeneratable)"
	}
	return ""
}

// classifyStream determines the type of stream for the manifest
func classifyStream(name string) string {
	if strings.HasPrefix(name, "KV_") {
		return "kv"
	}
	if strings.HasPrefix(name, "OBJ_") {
		return "object_store"
	}
	return "stream"
}

// createTarGz creates a .tar.gz archive from a directory.
func createTarGz(sourceDir, destFile string) error {
	return writeArchiveAtomically(destFile, func(outFile io.Writer) error {
		return writeTarGz(sourceDir, outFile)
	})
}

// createEncryptedTarGz creates an age-encrypted .tar.gz archive.
func createEncryptedTarGz(sourceDir, destFile, passphrase string) error {
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("failed to create age recipient: %w", err)
	}

	return writeArchiveAtomically(destFile, func(outFile io.Writer) error {
		ageWriter, err := age.Encrypt(outFile, recipient)
		if err != nil {
			return fmt.Errorf("failed to initialize encryption: %w", err)
		}
		if err := writeTarGz(sourceDir, ageWriter); err != nil {
			return err
		}
		if err := ageWriter.Close(); err != nil {
			return fmt.Errorf("failed to finalize encryption: %w", err)
		}
		return nil
	})
}

func writeArchiveAtomically(destFile string, write func(io.Writer) error) (err error) {
	tempFile, err := os.CreateTemp(filepath.Dir(destFile), "."+filepath.Base(destFile)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary archive file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err = tempFile.Chmod(0600); err != nil {
		return fmt.Errorf("failed to secure temporary archive file: %w", err)
	}
	if err = write(tempFile); err != nil {
		return err
	}
	if err = tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync archive file: %w", err)
	}
	if err = tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close archive file: %w", err)
	}
	if err = os.Rename(tempPath, destFile); err != nil {
		return fmt.Errorf("failed to publish archive atomically: %w", err)
	}
	parent, err := os.Open(filepath.Dir(destFile))
	if err != nil {
		return fmt.Errorf("failed to open archive directory for sync: %w", err)
	}
	if err = parent.Sync(); err != nil {
		parent.Close()
		return fmt.Errorf("failed to sync archive directory: %w", err)
	}
	if err = parent.Close(); err != nil {
		return fmt.Errorf("failed to close archive directory: %w", err)
	}
	return nil
}

// writeTarGz writes tar.gz content from a source directory to the provided writer.
func writeTarGz(sourceDir string, w io.Writer) error {
	gzWriter := gzip.NewWriter(w)
	tarWriter := tar.NewWriter(gzWriter)

	baseName := filepath.Base(sourceDir)

	if err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", path, err)
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		if relPath == "." {
			header.Name = baseName + "/"
		} else {
			header.Name = filepath.Join(baseName, relPath)
		}

		if info.IsDir() {
			header.Name += "/"
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", path, err)
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			if _, err := io.Copy(tarWriter, file); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file content for %s: %w", path, err)
			}
			if err := file.Close(); err != nil {
				return fmt.Errorf("failed to close file %s: %w", path, err)
			}
		}

		return nil
	}); err != nil {
		return err
	}
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize tar archive: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to finalize gzip archive: %w", err)
	}
	return nil
}

// extractTarGz extracts a .tar.gz archive into a destination directory.
func extractTarGz(archiveFile, destDir string) error {
	inFile, err := os.Open(archiveFile)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer inFile.Close()

	return readTarGz(inFile, destDir)
}

// readTarGz extracts tar.gz content from a reader into a destination directory.
func readTarGz(r io.Reader, destDir string) error {
	return readTarGzWithLimits(r, destDir, maxRestoreArchiveEntries, maxRestoreArchiveFileBytes, maxRestoreArchiveExpandedBytes)
}

func readTarGzWithLimits(r io.Reader, destDir string, maxEntries int, maxFileBytes, maxTotalBytes int64) error {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	entries := 0
	var expandedBytes int64
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}
		entries++
		if entries > maxEntries {
			return fmt.Errorf("archive contains more than %d entries", maxEntries)
		}

		// Prevent path traversal attacks
		target := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(destDir) {
			return fmt.Errorf("invalid tar entry path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxFileBytes {
				return fmt.Errorf("tar entry %q exceeds the per-file extraction limit", header.Name)
			}
			if header.Size > maxTotalBytes-expandedBytes {
				return fmt.Errorf("archive exceeds the total extraction limit of %d bytes", maxTotalBytes)
			}
			expandedBytes += header.Size
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", target, err)
			}
			outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("failed to close file %s: %w", target, err)
			}
		default:
			return fmt.Errorf("unsupported tar entry type %d for %q", header.Typeflag, header.Name)
		}
	}

	return nil
}

// formatBytes formats a byte count for human readable output
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
