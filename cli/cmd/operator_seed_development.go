//go:build bootstrap || test_endpoints

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/assets"
	"github.com/chattocorp/chatto/cli/internal/core"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1/operatorv1connect"
	"github.com/spf13/cobra"
)

func init() { operatorCmd.AddCommand(operatorSeedCmd()) }

// operatorSeedCmd uses the running development server's local operator socket.
// It never opens a second NATS writer or retries a partially committed run.
func operatorSeedCmd() *cobra.Command {
	var request operatorv1.SeedDataRequest
	var timeout time.Duration
	var avatarDir string
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Create synthetic users, rooms, and messages (development builds)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if timeout <= 0 {
				return fmt.Errorf("timeout must be positive")
			}
			images, err := readSeedAvatarImages(avatarDir)
			if err != nil {
				return err
			}
			request.AvatarImages = images
			options := core.SeedOptions{
				Seed:  request.Seed,
				Users: int(request.Users), Rooms: int(request.Rooms),
				Messages: int(request.Messages), ThreadReplies: int(request.ThreadReplies),
				AvatarImages: images,
			}
			if err := options.Validate(); err != nil {
				return err
			}
			httpClient, baseURL, err := newOperatorHTTPClient()
			if err != nil {
				return err
			}
			defer httpClient.CloseIdleConnections()
			client := operatorv1connect.NewOperatorSeedServiceClient(httpClient, baseURL)
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			result, err := client.SeedData(ctx, connect.NewRequest(&request))
			if err != nil {
				return seedCommandError(err)
			}
			return printOperatorOutput(cmd.OutOrStdout(), result.Msg, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Created %d users, %d rooms, and %d messages (seed %d, %s).\n", len(result.Msg.Users), len(result.Msg.Rooms), len(result.Msg.Messages), result.Msg.Seed, result.Msg.Version)
			})
		},
	}
	cmd.Flags().Int64Var(&request.Seed, "seed", 1, "random seed for reproducible content and relationships")
	cmd.Flags().Int32Var(&request.Users, "users", 20, "number of passwordless synthetic accounts")
	cmd.Flags().Int32Var(&request.Rooms, "rooms", 5, "number of channels")
	cmd.Flags().Int32Var(&request.Messages, "messages", 200, "total posts, including thread replies")
	cmd.Flags().Int32Var(&request.ThreadReplies, "thread-replies", 0, "posts that reply to generated root messages")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for seeding")
	cmd.Flags().StringVar(&avatarDir, "avatar-dir", "", "optional folder of JPEG, PNG, or WebP avatars (sorted by filename, resized and cycled across users; 50 files and 512 KiB converted total maximum; no network requests)")
	return cmd
}

// readSeedAvatarImages reads a bounded, filename-sorted pool before any seed writes.
// Symlinks and non-image files are skipped; an explicitly selected empty folder fails.
func readSeedAvatarImages(directory string) ([][]byte, error) {
	if directory == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read avatar directory: %w", err)
	}
	var images [][]byte
	remaining := int64(core.MaxSeedAvatarBytes)
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".jpg", ".jpeg", ".png", ".webp":
		default:
			continue
		}
		if len(images) == 50 {
			return nil, fmt.Errorf("at most 50 seed avatar images are allowed")
		}
		file, err := os.Open(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("open seed avatar: %w", err)
		}
		processed, readErr := assets.ProcessAvatarImage(file)
		closeErr := file.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, fmt.Errorf("read seed avatar: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(processed, remaining+1))
		if err != nil {
			return nil, fmt.Errorf("read processed seed avatar: %w", err)
		}
		remaining -= int64(len(data))
		if remaining < 0 {
			return nil, fmt.Errorf("seed avatar images must not exceed 512 KiB in total")
		}
		images = append(images, data)
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("avatar directory contains no JPEG, PNG, or WebP files")
	}
	return images, nil
}

// A missing or refused Unix-socket dial cannot have sent the seed request.
// Errors after connection remain ambiguous: earlier operations may have committed.
func seedCommandError(err error) error {
	var dial *net.OpError
	if errors.As(err, &dial) && dial.Op == "dial" && dial.Net == "unix" &&
		(errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED)) {
		return fmt.Errorf("cannot connect to the development server; start or restart mise dev, then run mise seed again (no data was created): %w", err)
	}
	return fmt.Errorf("seed failed; partial data may remain (do not retry automatically): %w", err)
}
