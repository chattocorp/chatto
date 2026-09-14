//go:build bootstrap || test_endpoints

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/core"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

func init() { operatorCmd.AddCommand(operatorSeedCmd()) }

// operatorSeedCmd uses the running development server's local operator socket.
// It never opens a second NATS writer or retries a partially committed run.
func operatorSeedCmd() *cobra.Command {
	var request operatorv1.SeedDataRequest
	var timeout time.Duration
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
			}
			if err := options.Validate(); err != nil {
				return err
			}
			resolved, err := resolveOperatorAPIClientConfig()
			if err != nil {
				return err
			}
			transport := newOperatorSocketTransport(resolved.socketPath)
			defer transport.CloseIdleConnections()
			client := operatorv1connect.NewOperatorSeedServiceClient(&http.Client{Transport: transport}, resolved.connectBaseURL)
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			result, err := client.SeedData(ctx, connect.NewRequest(&request))
			if err != nil {
				return fmt.Errorf("seed failed; partial data may remain (do not retry automatically): %w", err)
			}
			return printAdminOutput(cmd.OutOrStdout(), result.Msg, func() {
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
	return cmd
}
