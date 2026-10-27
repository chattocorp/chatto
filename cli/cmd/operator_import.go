package cmd

import (
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/interchange"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1/operatorv1connect"
	"github.com/spf13/cobra"
)

func init() { operatorCmd.AddCommand(newOperatorImportCommand()) }

// newOperatorImportCommand validates local bytes before sending them through
// the operator socket. Destination policy and durable recovery belong to core.
func newOperatorImportCommand() *cobra.Command {
	var validateOnly, acceptOmissions bool
	var naming operatorv1.ImportNaming
	command := &cobra.Command{
		Use: "import PATH", Short: "Import a portable history archive (draft v1)",
		Long: "Import or resume a draft v1 archive through the local operator socket. Keep the destination unused until import completes. Reuse the same archive and naming options when retrying.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			archive, err := interchange.ReadDirectory(args[0])
			if err != nil {
				return err
			}
			client, baseURL, err := newOperatorHTTPClient()
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			response, err := operatorv1connect.NewOperatorImportServiceClient(client, baseURL).ImportArchive(cmd.Context(), operatorRequest(&operatorv1.ImportArchiveRequest{
				Metadata: archive.Metadata, Files: archive.Files, Naming: &naming, ValidateOnly: validateOnly, AcceptOmissions: acceptOmissions,
			}))
			if err != nil {
				return err
			}
			return printOperatorOutput(cmd.OutOrStdout(), response.Msg, func() {
				state := "Archive and destination validated; no data imported"
				if !validateOnly {
					state = "Import complete"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %d users, %d channels, %d assets, %d events; %d declared omissions.\n", state, response.Msg.Users, response.Msg.Rooms, response.Msg.Assets, response.Msg.Events, response.Msg.Omissions)
			})
		},
	}
	command.Flags().BoolVar(&validateOnly, "validate-only", false, "check archive and destination without writes")
	command.Flags().BoolVar(&acceptOmissions, "accept-omissions", false, "accept losses declared in the archive")
	command.Flags().StringVar(&naming.ChannelPrefix, "channel-prefix", "", "prefix imported channel names")
	command.Flags().StringVar(&naming.ChannelSuffix, "channel-suffix", "", "suffix imported channel names")
	command.Flags().StringVar(&naming.UsernamePrefix, "username-prefix", "", "prefix imported usernames")
	command.Flags().StringVar(&naming.UsernameSuffix, "username-suffix", "", "suffix imported usernames")
	return command
}
