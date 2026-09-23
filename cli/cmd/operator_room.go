package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

var operatorRoomCmd = &cobra.Command{
	Use:   "room",
	Short: "Find and create channel rooms through the local operator API",
}

func init() {
	operatorCmd.AddCommand(operatorRoomCmd)
	operatorRoomCmd.AddCommand(operatorRoomListCmd())
	operatorRoomCmd.AddCommand(operatorRoomCreateCmd())
}

func operatorRoomCreateCmd() *cobra.Command {
	var name, description, groupID string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a channel room",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" {
				return errors.New("--name is required")
			}
			client, err := newOperatorRoomClient()
			if err != nil {
				return err
			}
			resp, err := client.CreateRoom(cmd.Context(), operatorRequest(&operatorv1.CreateRoomRequest{
				Name: name, Description: description, GroupId: groupID,
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() {
				room := resp.Msg.GetRoom()
				fmt.Fprintf(out, "%s\t%s\tgroup=%s\tarchived=%t\tdescription=%q\n", room.GetId(), room.GetName(), room.GetGroupId(), room.GetArchived(), room.GetDescription())
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "channel name")
	cmd.Flags().StringVar(&description, "description", "", "channel description")
	cmd.Flags().StringVar(&groupID, "group-id", "", "room group ID (default: first group)")
	return cmd
}

func newOperatorRoomClient() (operatorv1connect.OperatorRoomServiceClient, error) {
	httpClient, baseURL, err := newOperatorHTTPClient()
	if err != nil {
		return nil, err
	}
	return operatorv1connect.NewOperatorRoomServiceClient(httpClient, baseURL), nil
}

func operatorRoomListCmd() *cobra.Command {
	var name string
	var limit int32
	var offset int32
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List channel rooms, including archived rooms",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if offset < 0 {
				return errors.New("--offset must be greater than or equal to 0")
			}
			requestLimit := limit
			if requestLimit < 0 {
				requestLimit = 0
			}
			if requestLimit > 100 {
				requestLimit = 100
			}
			client, err := newOperatorRoomClient()
			if err != nil {
				return err
			}
			resp, err := client.ListRooms(cmd.Context(), operatorRequest(&operatorv1.ListRoomsRequest{
				Name: name,
				Page: &apiv1.PageRequest{Limit: requestLimit, Offset: offset},
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() {
				for _, room := range resp.Msg.GetRooms() {
					fmt.Fprintf(out, "%s\t%s\tgroup=%s\tarchived=%t\tdescription=%q\n", room.GetId(), room.GetName(), room.GetGroupId(), room.GetArchived(), room.GetDescription())
				}
				page := resp.Msg.GetPage()
				fmt.Fprintf(out, "total=%d has_more=%t\n", page.GetTotalCount(), page.GetHasMore())
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "exact stored channel name")
	cmd.Flags().Int32Var(&limit, "limit", 20, "maximum rooms to return")
	cmd.Flags().Int32Var(&offset, "offset", 0, "zero-based result offset")
	return cmd
}
