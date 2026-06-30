package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"

	"connectrpc.com/connect"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

var operatorConfigFile string
var operatorSocketPath string
var operatorOutputJSON bool

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Local operator commands",
}

var operatorUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users through the local operator API",
}

func init() {
	rootCmd.AddCommand(operatorCmd)
	operatorCmd.AddCommand(operatorUserCmd)
	operatorCmd.PersistentFlags().StringVarP(&operatorConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	operatorCmd.PersistentFlags().StringVar(&operatorSocketPath, "operator-socket", "", "operator API Unix socket path")
	operatorCmd.PersistentFlags().BoolVar(&operatorOutputJSON, "json", false, "print JSON output")

	operatorUserCmd.AddCommand(
		adminUserListCmd(),
		adminUserGetCmd(),
		adminUserCreateCmd(),
		adminUserUpdateCmd(),
		adminUserSetPasswordCmd(),
		adminUserDeleteCmd(),
		adminUserAddEmailCmd(),
		adminUserRoleCmd(),
	)
}

func adminUserListCmd() *cobra.Command {
	var search string
	var limit int32
	var offset int32
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			requestLimit := limit
			if requestLimit < 0 {
				requestLimit = 0
			}
			if requestLimit > 100 {
				requestLimit = 100
			}
			if offset < 0 {
				return errors.New("--offset must be greater than or equal to 0")
			}
			resp, err := client.ListUsers(cmd.Context(), adminRequest(&operatorv1.ListUsersRequest{
				Search: search,
				Page: &apiv1.PageRequest{
					Limit:  requestLimit,
					Offset: offset,
				},
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() {
				for _, user := range resp.Msg.GetUsers() {
					printAdminMemberLine(out, user)
				}
				page := resp.Msg.GetPage()
				totalCount := page.GetTotalCount()
				hasMore := page.GetHasMore()
				fmt.Fprintf(out, "total=%d has_more=%t\n", totalCount, hasMore)
			})
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "search login/display name or exact verified email")
	cmd.Flags().Int32Var(&limit, "limit", 20, "maximum users to return")
	cmd.Flags().Int32Var(&offset, "offset", 0, "zero-based result offset")
	return cmd
}

func adminUserGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get USER_ID",
		Short: "Get a user by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.GetUser(cmd.Context(), adminRequest(&operatorv1.GetUserRequest{UserId: args[0]}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	return cmd
}

func adminUserCreateCmd() *cobra.Command {
	var login string
	var displayName string
	var password string
	var passwordFile string
	var passwordStdin bool
	var verifiedEmail string
	var roles []string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(login) == "" {
				return errors.New("--login is required")
			}
			passwordSet := cmd.Flags().Changed("password") || passwordFile != "" || passwordStdin
			if err := validateSecretSources("--password", cmd.Flags().Changed("password"), "--password-file", passwordFile != "", "--password-stdin", passwordStdin); err != nil {
				return err
			}
			if passwordFile != "" {
				fromFile, err := readSecretFile(passwordFile)
				if err != nil {
					return err
				}
				password = fromFile
			}
			if passwordStdin {
				fromStdin, err := readSecretStdin()
				if err != nil {
					return err
				}
				password = fromStdin
			}
			if !passwordSet && term.IsTerminal(int(syscall.Stdin)) {
				prompted, err := readPassword("Password (leave empty for no password): ")
				if err != nil {
					return err
				}
				password = prompted
			}
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.CreateUser(cmd.Context(), adminRequest(&operatorv1.CreateUserRequest{
				Login:         login,
				DisplayName:   displayName,
				Password:      password,
				VerifiedEmail: verifiedEmail,
				RoleNames:     roles,
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "login for the new user")
	cmd.Flags().StringVar(&displayName, "display-name", "", "display name; defaults to login")
	cmd.Flags().StringVar(&password, "password", "", "password for the new user; prefer --password-stdin or --password-file for automation")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "file containing the password for the new user")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the password for the new user from stdin")
	cmd.Flags().StringVar(&verifiedEmail, "verified-email", "", "email to add as already verified")
	cmd.Flags().StringArrayVar(&roles, "role", nil, "role to assign; repeatable")
	return cmd
}

func adminUserUpdateCmd() *cobra.Command {
	var newLogin string
	var displayName string
	cmd := &cobra.Command{
		Use:   "update USER_ID",
		Short: "Update a user's profile fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("new-login") && !cmd.Flags().Changed("display-name") {
				return errors.New("provide --new-login and/or --display-name")
			}
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			req := &operatorv1.UpdateUserRequest{UserId: args[0]}
			if cmd.Flags().Changed("new-login") {
				req.Login = &newLogin
			}
			if cmd.Flags().Changed("display-name") {
				req.DisplayName = &displayName
			}
			resp, err := client.UpdateUser(cmd.Context(), adminRequest(req))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&newLogin, "new-login", "", "new login")
	cmd.Flags().StringVar(&displayName, "display-name", "", "new display name")
	return cmd
}

func adminUserSetPasswordCmd() *cobra.Command {
	var password string
	var passwordFile string
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:     "set-password USER_ID",
		Aliases: []string{"setpassword"},
		Short:   "Set a user's password",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateSecretSources("--password", cmd.Flags().Changed("password"), "--password-file", passwordFile != "", "--password-stdin", passwordStdin); err != nil {
				return err
			}
			if passwordFile != "" {
				fromFile, err := readSecretFile(passwordFile)
				if err != nil {
					return err
				}
				password = fromFile
			}
			if passwordStdin {
				fromStdin, err := readSecretStdin()
				if err != nil {
					return err
				}
				password = fromStdin
			}
			if !cmd.Flags().Changed("password") && passwordFile == "" && !passwordStdin {
				if !term.IsTerminal(int(syscall.Stdin)) {
					return errors.New("--password, --password-file, or --password-stdin is required when stdin is not a terminal")
				}
				var err error
				password, err = readRequiredPassword("New password: ")
				if err != nil {
					return err
				}
			}
			if password == "" {
				return errors.New("password cannot be empty")
			}
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.SetUserPassword(cmd.Context(), adminRequest(&operatorv1.SetUserPasswordRequest{
				UserId:   args[0],
				Password: password,
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&password, "password", "", "new password; prefer --password-stdin or --password-file for automation")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "file containing the new password")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the new password from stdin")
	return cmd
}

func adminUserDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete USER_ID",
		Short: "Permanently delete a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			if !yes {
				if !term.IsTerminal(int(syscall.Stdin)) {
					return errors.New("--yes is required when stdin is not a terminal")
				}
				if err := confirmDeletion(args[0]); err != nil {
					return err
				}
			}
			resp, err := client.DeleteUser(cmd.Context(), adminRequest(&operatorv1.DeleteUserRequest{UserId: args[0]}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { fmt.Fprintf(out, "deleted user %s\n", args[0]) })
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm irreversible user deletion")
	return cmd
}

func adminUserAddEmailCmd() *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "add-email USER_ID --email EMAIL",
		Short: "Add a verified email address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(email) == "" {
				return errors.New("--email is required")
			}
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.AddVerifiedEmail(cmd.Context(), adminRequest(&operatorv1.AddVerifiedEmailRequest{
				UserId: args[0],
				Email:  email,
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to add as already verified")
	return cmd
}

func adminUserRoleCmd() *cobra.Command {
	roleCmd := &cobra.Command{
		Use:   "role",
		Short: "Manage user roles",
	}
	addCmd := &cobra.Command{
		Use:   "add USER_ID ROLE",
		Short: "Assign a role",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.AssignRole(cmd.Context(), adminRequest(&operatorv1.AssignRoleRequest{
				UserId:   args[0],
				RoleName: args[1],
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	roleCmd.AddCommand(addCmd)

	removeCmd := &cobra.Command{
		Use:     "remove USER_ID ROLE",
		Aliases: []string{"rm"},
		Short:   "Revoke a role",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.RevokeRole(cmd.Context(), adminRequest(&operatorv1.RevokeRoleRequest{
				UserId:   args[0],
				RoleName: args[1],
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	roleCmd.AddCommand(removeCmd)
	return roleCmd
}

func newOperatorAPIClient() (operatorv1connect.OperatorUserServiceClient, error) {
	resolved, err := resolveOperatorAPIClientConfig()
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: newOperatorSocketTransport(resolved.socketPath)}
	return operatorv1connect.NewOperatorUserServiceClient(httpClient, resolved.connectBaseURL), nil
}

type resolvedOperatorAPIConfig struct {
	connectBaseURL string
	socketPath     string
}

func resolveOperatorAPIClientConfig() (resolvedOperatorAPIConfig, error) {
	resolved := resolvedOperatorAPIConfig{
		connectBaseURL: "http://chatto-operator" + connectapi.Prefix,
		socketPath:     strings.TrimSpace(operatorSocketPath),
	}
	if envSocketPath := strings.TrimSpace(os.Getenv("CHATTO_OPERATOR_API_SOCKET_PATH")); resolved.socketPath == "" && envSocketPath != "" {
		resolved.socketPath = envSocketPath
	}
	cfg, cfgErr := readOperatorConfigFile(operatorConfigFile)
	if cfgErr != nil {
		return resolved, cfgErr
	}
	if resolved.socketPath == "" {
		resolved.socketPath = cfg.OperatorAPI.SocketPathOrDefault()
	}
	return resolved, nil
}

func readOperatorConfigFile(path string) (config.ChattoConfig, error) {
	var cfg config.ChattoConfig
	if path == "" {
		path = "chatto.toml"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && operatorConfigFile == "" {
			return cfg, nil
		}
		return cfg, err
	}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func validateSecretSources(sources ...any) error {
	var set []string
	for i := 0; i+1 < len(sources); i += 2 {
		name, _ := sources[i].(string)
		isSet, _ := sources[i+1].(bool)
		if isSet {
			set = append(set, name)
		}
	}
	if len(set) > 1 {
		return fmt.Errorf("provide only one of %s", strings.Join(set, ", "))
	}
	return nil
}

func readSecretFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return trimSecretNewline(string(b)), nil
}

func readSecretStdin() (string, error) {
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return trimSecretNewline(string(b)), nil
}

func trimSecretNewline(s string) string {
	return strings.TrimRight(s, "\r\n")
}

func newOperatorSocketTransport(socketPath string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
}

func adminRequest[T any](msg *T) *connect.Request[T] {
	return connect.NewRequest(msg)
}

func printAdminOutput(out io.Writer, message proto.Message, human func()) error {
	if operatorOutputJSON {
		b, err := protojson.MarshalOptions{Indent: "  "}.Marshal(message)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, string(b))
		return nil
	}
	human()
	return nil
}

func printAdminMemberLine(out io.Writer, member *adminv1.AdminMember) {
	if member == nil || member.GetUser() == nil {
		return
	}
	user := member.GetUser()
	roles := strings.Join(member.GetRoles(), ",")
	if roles == "" {
		roles = "-"
	}
	emailText := strings.Join(member.GetVerifiedEmails(), ",")
	if emailText == "" {
		emailText = "-"
	}
	fmt.Fprintf(out, "%s\t%s\t%s\troles=%s\temails=%s\n", user.GetId(), user.GetLogin(), user.GetDisplayName(), roles, emailText)
}

func readPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	pass, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(pass), nil
}

func readRequiredPassword(prompt string) (string, error) {
	pass, err := readPassword(prompt)
	if err != nil {
		return "", err
	}
	if pass == "" {
		return "", errors.New("password cannot be empty")
	}
	return pass, nil
}

func confirmDeletion(userID string) error {
	fmt.Fprintf(os.Stderr, "Type DELETE %s to permanently delete this user: ", userID)
	confirmation, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirmation) != "DELETE "+userID {
		return errors.New("delete confirmation did not match")
	}
	return nil
}
