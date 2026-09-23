package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

var operatorRoomMemberCmd = &cobra.Command{
	Use:   "member",
	Short: "Manage explicit channel room members",
}

func init() {
	operatorRoomCmd.AddCommand(operatorRoomMemberCmd)
	operatorRoomMemberCmd.AddCommand(operatorRoomMemberAddCmd())
}

func operatorRoomMemberAddCmd() *cobra.Command {
	var roomID, userID string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a user to a channel room",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(roomID) == "" || strings.TrimSpace(userID) == "" {
				return errors.New("--room-id and --user-id are required")
			}
			client, err := newOperatorRoomClient()
			if err != nil {
				return err
			}
			resp, err := client.AddMember(cmd.Context(), operatorRequest(&operatorv1.AddMemberRequest{
				RoomId: roomID, UserId: userID,
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() {
				fmt.Fprintf(out, "room=%s\tuser=%s\n", resp.Msg.GetRoomId(), resp.Msg.GetMember().GetUser().GetId())
			})
		},
	}
	cmd.Flags().StringVar(&roomID, "room-id", "", "channel room ID")
	cmd.Flags().StringVar(&userID, "user-id", "", "user ID")
	return cmd
}
