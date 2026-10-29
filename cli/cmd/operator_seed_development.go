//go:build bootstrap || test_endpoints

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	var pravatar bool
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Create synthetic users, rooms, and messages (development builds)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if timeout <= 0 {
				return fmt.Errorf("timeout must be positive")
			}
			options := core.SeedOptions{
				Seed:  request.Seed,
				Users: int(request.Users), Rooms: int(request.Rooms),
				Messages: int(request.Messages), ThreadReplies: int(request.ThreadReplies),
				NotificationRecipient: request.NotificationRecipient, Notifications: int(request.Notifications), NotificationDays: int(request.NotificationDays),
			}
			if err := options.Validate(); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			images, err := readSeedAvatarImages(avatarDir)
			if pravatar {
				remote := &http.Client{Timeout: 20 * time.Second, CheckRedirect: seedAvatarRedirect}
				defer remote.CloseIdleConnections()
				images, err = downloadSeedAvatars(ctx, remote, request.Seed, int(request.Users))
			}
			if err != nil {
				return fmt.Errorf("prepare seed avatars (no data was created): %w", err)
			}
			request.AvatarImages = images
			options.AvatarImages = images
			if err := options.Validate(); err != nil {
				return err
			}
			httpClient, baseURL, err := newOperatorHTTPClient()
			if err != nil {
				return err
			}
			defer httpClient.CloseIdleConnections()
			client := operatorv1connect.NewOperatorSeedServiceClient(httpClient, baseURL)
			result, err := client.SeedData(ctx, connect.NewRequest(&request))
			if err != nil {
				return seedCommandError(err)
			}
			return printOperatorOutput(cmd.OutOrStdout(), result.Msg, func() {
				fmt.Fprintf(cmd.OutOrStdout(), "Created %d users, %d rooms, and %d messages (seed %d, %s).\n", len(result.Msg.Users), len(result.Msg.Rooms), len(result.Msg.Messages), result.Msg.Seed, result.Msg.Version)
				if result.Msg.NotificationCount > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Created %d notification fixtures.\n", result.Msg.NotificationCount)
				}
			})
		},
	}
	cmd.Flags().Int64Var(&request.Seed, "seed", 1, "random seed for reproducible content and relationships")
	cmd.Flags().Int32Var(&request.Users, "users", 20, "number of passwordless synthetic accounts")
	cmd.Flags().Int32Var(&request.Rooms, "rooms", 5, "number of channels")
	cmd.Flags().Int32Var(&request.Messages, "messages", 200, "total posts, including thread replies")
	cmd.Flags().Int32Var(&request.ThreadReplies, "thread-replies", 0, "posts that reply to generated root messages")
	cmd.Flags().StringVar(&request.NotificationRecipient, "notification-recipient", "", "existing login that receives notification fixtures")
	cmd.Flags().Int32Var(&request.Notifications, "notifications", 0, "additional historical notification fixtures (0-1000)")
	cmd.Flags().Int32Var(&request.NotificationDays, "notification-days", 5, "days of notification history, including today (1-30)")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "maximum time to wait for seeding")
	cmd.Flags().StringVar(&avatarDir, "avatar-dir", "", "optional folder of JPEG, PNG, or WebP avatars (sorted by filename, resized and cycled across users; 50 files and 512 KiB converted total maximum; no network requests)")
	cmd.Flags().BoolVar(&pravatar, "pravatar", false, "download up to 20 portraits from i.pravatar.cc before seeding (shares your IP and synthetic seed keys with Pravatar; stored locally)")
	cmd.MarkFlagsMutuallyExclusive("avatar-dir", "pravatar")
	return cmd
}

// seedAvatarRedirect keeps provider redirects on its HTTPS image origin.
func seedAvatarRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 || req.URL.Scheme != "https" || req.URL.Host != "i.pravatar.cc" {
		return fmt.Errorf("unexpected Pravatar redirect")
	}
	return nil
}

// downloadSeedAvatars prepares a bounded pool before the first durable seed write.
// Keys contain only the random seed and index, never account identifiers. Larger
// datasets cycle this pool through the same path as local avatar files.
func downloadSeedAvatars(ctx context.Context, client *http.Client, seed int64, users int) ([][]byte, error) {
	images := make([][]byte, 0, min(users, 20))
	remaining := int64(core.MaxSeedAvatarBytes)
	for i := range min(users, 20) {
		url := fmt.Sprintf("https://i.pravatar.cc/128?u=chatto-seed-%d-%d", seed, i)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "image/jpeg,image/png,image/webp")
		response, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("download Pravatar image %d: %w", i+1, err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("Pravatar image %d returned HTTP %d", i+1, response.StatusCode)
		}
		processed, processErr := assets.ProcessAvatarImageWithConfig(response.Body, assets.Config{MaxUploadSize: 1 << 20})
		closeErr := response.Body.Close()
		if err := errors.Join(processErr, closeErr); err != nil {
			return nil, fmt.Errorf("decode Pravatar image %d: %w", i+1, err)
		}
		data, err := io.ReadAll(io.LimitReader(processed, remaining+1))
		if err != nil {
			return nil, err
		}
		remaining -= int64(len(data))
		if remaining < 0 {
			return nil, fmt.Errorf("Pravatar images exceed the 512 KiB seed avatar limit")
		}
		images = append(images, data)
	}
	return images, nil
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
