package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	"github.com/spf13/cobra"
)

func init() { operatorCmd.AddCommand(newOperatorImportCommand()) }

type importNaming struct {
	ChannelPrefix, ChannelSuffix, UsernamePrefix, UsernameSuffix string
	// Omit the empty option to retain existing archive journal bindings.
	BotOwner string `json:",omitempty"`
}
type importResult struct {
	Users     int  `json:"users"`
	Rooms     int  `json:"rooms"`
	Assets    int  `json:"assets"`
	Events    int  `json:"events"`
	Omissions int  `json:"omissions"`
	Complete  bool `json:"complete"`
}

// newOperatorImportCommand owns archive interpretation. The server receives
// individual resource operations, never an archive or source paths.
func newOperatorImportCommand() *cobra.Command {
	var validateOnly, acceptOmissions bool
	var naming importNaming
	var progress string
	c := &cobra.Command{Use: "import PATH", Short: "Import a portable history archive (draft v1)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		a, err := interchange.ReadDirectory(args[0])
		if err != nil {
			return err
		}
		defer a.Close()
		m, err := newCLIArchiveImport(a, naming, progress)
		if err != nil {
			return err
		}
		defer m.close()
		if err = m.run(cmd.Context(), validateOnly, acceptOmissions); err != nil {
			return err
		}
		r := importResult{Users: len(a.Users), Rooms: len(a.Rooms), Assets: len(a.Assets), Events: a.EventCount, Omissions: len(a.Manifest.Omissions), Complete: !validateOnly}
		if operatorOutputJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(r)
		}
		state := "Import complete"
		if validateOnly {
			state = "Archive and destination validated; no data imported"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %d users, %d channels, %d assets, %d events; %d declared omissions.\n", state, r.Users, r.Rooms, r.Assets, r.Events, r.Omissions)
		return err
	}}
	c.Flags().BoolVar(&validateOnly, "validate-only", false, "check archive and destination without server writes")
	c.Flags().BoolVar(&acceptOmissions, "accept-omissions", false, "accept losses declared in the archive")
	c.Flags().StringVar(&progress, "progress", "", "private progress journal (default: user cache directory)")
	c.Flags().StringVar(&naming.ChannelPrefix, "channel-prefix", "", "prefix imported channel names")
	c.Flags().StringVar(&naming.ChannelSuffix, "channel-suffix", "", "suffix imported channel names")
	c.Flags().StringVar(&naming.UsernamePrefix, "username-prefix", "", "prefix imported usernames")
	c.Flags().StringVar(&naming.UsernameSuffix, "username-suffix", "", "suffix imported usernames")
	c.Flags().StringVar(&naming.BotOwner, "bot-owner", "", "override the server owner for imported bots with an existing active human username")
	return c
}
