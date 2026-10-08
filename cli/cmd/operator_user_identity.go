package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func operatorUserIdentityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "identity", Short: "Manage external sign-in identities for existing users",
	}
	cmd.AddCommand(operatorUserIdentityListCmd(), operatorUserIdentityLinkCmd(), operatorUserIdentityUnlinkCmd())
	return cmd
}

func operatorUserIdentityListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list USER_ID", Short: "List external identities, including stale links",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("USER_ID must not be empty")
			}
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.ListUserExternalIdentities(cmd.Context(), operatorRequest(&operatorv1.ListUserExternalIdentitiesRequest{UserId: args[0]}))
			if err != nil {
				return err
			}
			return printOperatorOutput(cmd.OutOrStdout(), resp.Msg, func() {
				for _, identity := range resp.Msg.GetIdentities() {
					printOperatorIdentity(cmd.OutOrStdout(), identity)
				}
			})
		},
	}
}

func operatorUserIdentityLinkCmd() *cobra.Command {
	var providerID, subject string
	cmd := &cobra.Command{
		Use:   "link USER_ID --provider PROVIDER_ID --subject SUBJECT",
		Short: "Link an operator-verified provider subject to an existing user",
		Long:  "Link an operator-verified provider subject to an existing user. Verify ownership first. The server uses the configured provider's current issuer; this command does not contact the provider or match by email.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("USER_ID must not be empty")
			}
			if strings.TrimSpace(providerID) == "" || strings.TrimSpace(subject) == "" {
				return fmt.Errorf("--provider and --subject must not be empty")
			}
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.LinkUserExternalIdentity(cmd.Context(), operatorRequest(&operatorv1.LinkUserExternalIdentityRequest{
				UserId: args[0], ProviderId: providerID, Subject: subject,
			}))
			if err != nil {
				return err
			}
			return printOperatorOutput(cmd.OutOrStdout(), resp.Msg, func() { printOperatorIdentity(cmd.OutOrStdout(), resp.Msg.GetIdentity()) })
		},
	}
	cmd.Flags().StringVar(&providerID, "provider", "", "configured provider ID")
	cmd.Flags().StringVar(&subject, "subject", "", "exact provider subject whose ownership you verified")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("subject")
	return cmd
}

func operatorUserIdentityUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use: "unlink USER_ID SUBJECT_HASH", Short: "Remove an external identity while retaining another sign-in method",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
				return fmt.Errorf("USER_ID and SUBJECT_HASH must not be empty")
			}
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.UnlinkUserExternalIdentity(cmd.Context(), operatorRequest(&operatorv1.UnlinkUserExternalIdentityRequest{
				UserId: args[0], SubjectHash: args[1],
			}))
			if err != nil {
				return err
			}
			return printOperatorOutput(cmd.OutOrStdout(), resp.Msg, func() { fmt.Fprintf(cmd.OutOrStdout(), "unlinked identity %s from user %s\n", args[1], args[0]) })
		},
	}
}

func printOperatorIdentity(out io.Writer, identity *operatorv1.UserExternalIdentity) {
	// Quote externally supplied strings so control characters cannot create
	// additional terminal rows. These details are explicit root-only output,
	// never server log fields.
	fmt.Fprintf(out, "%s\tprovider=%q\ttype=%q\tissuer=%q\tsubject=%q\tlogin_available=%t\n",
		identity.GetSubjectHash(), identity.GetProviderId(), identity.GetProviderType(), identity.GetIssuer(), identity.GetSubject(), identity.GetLoginAvailable())
}
