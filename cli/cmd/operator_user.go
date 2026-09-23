package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

var operatorUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users through the local operator API",
}

func init() {
	operatorCmd.AddCommand(operatorUserCmd)
	operatorUserCmd.AddCommand(
		operatorUserListCmd(),
		operatorUserGetCmd(),
		operatorUserCreateCmd(),
		operatorUserUpdateCmd(),
		operatorUserSetPasswordCmd(),
		operatorUserClearUsernameCooldownCmd(),
		operatorUserDeleteCmd(),
		operatorUserAddEmailCmd(),
		operatorUserRoleCmd(),
	)
}

func newOperatorUserClient() (operatorv1connect.OperatorUserServiceClient, error) {
	httpClient, baseURL, err := newOperatorHTTPClient()
	if err != nil {
		return nil, err
	}
	return operatorv1connect.NewOperatorUserServiceClient(httpClient, baseURL), nil
}

func operatorUserListCmd() *cobra.Command {
	var search string
	var limit int32
	var offset int32
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorUserClient()
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
			resp, err := client.ListUsers(cmd.Context(), operatorRequest(&operatorv1.ListUsersRequest{
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
			return printOperatorOutput(out, resp.Msg, func() {
				for _, user := range resp.Msg.GetUsers() {
					printOperatorUserLine(out, user)
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

func operatorUserGetCmd() *cobra.Command {
	var login, email string
	cmd := &cobra.Command{
		Use:   "get [USER_ID]",
		Short: "Get a user by ID, login, or verified email",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			selectors := len(args)
			if cmd.Flags().Changed("login") {
				selectors++
			}
			if cmd.Flags().Changed("email") {
				selectors++
			}
			if selectors != 1 {
				return errors.New("provide exactly one of USER_ID, --login, or --email")
			}
			request := &operatorv1.GetUserRequest{}
			if len(args) == 1 {
				if strings.TrimSpace(args[0]) == "" {
					return errors.New("USER_ID must not be empty")
				}
				request.UserId = args[0]
			} else if cmd.Flags().Changed("login") {
				if strings.TrimSpace(login) == "" {
					return errors.New("--login must not be empty")
				}
				request.Login = login
			} else {
				if strings.TrimSpace(email) == "" {
					return errors.New("--email must not be empty")
				}
				request.Email = email
			}
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.GetUser(cmd.Context(), operatorRequest(request))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { printOperatorUserLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "find a user by exact login")
	cmd.Flags().StringVar(&email, "email", "", "find a user by exact verified email")
	return cmd
}

func operatorUserClearUsernameCooldownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-username-cooldown USER_ID",
		Short: "Allow a user to change their username again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return errors.New("USER_ID must not be empty")
			}
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.ClearUsernameCooldown(cmd.Context(), operatorRequest(&operatorv1.ClearUsernameCooldownRequest{UserId: args[0]}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { fmt.Fprintf(out, "cleared username cooldown for user %s\n", args[0]) })
		},
	}
}

func operatorUserCreateCmd() *cobra.Command {
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
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.CreateUser(cmd.Context(), operatorRequest(&operatorv1.CreateUserRequest{
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
			return printOperatorOutput(out, resp.Msg, func() { printOperatorUserLine(out, resp.Msg.GetMember()) })
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

func operatorUserUpdateCmd() *cobra.Command {
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
			client, err := newOperatorUserClient()
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
			resp, err := client.UpdateUser(cmd.Context(), operatorRequest(req))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { printOperatorUserLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&newLogin, "new-login", "", "new login")
	cmd.Flags().StringVar(&displayName, "display-name", "", "new display name")
	return cmd
}

func operatorUserSetPasswordCmd() *cobra.Command {
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
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.SetUserPassword(cmd.Context(), operatorRequest(&operatorv1.SetUserPasswordRequest{
				UserId:   args[0],
				Password: password,
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { printOperatorUserLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&password, "password", "", "new password; prefer --password-stdin or --password-file for automation")
	cmd.Flags().StringVar(&passwordFile, "password-file", "", "file containing the new password")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read the new password from stdin")
	return cmd
}

func operatorUserDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete USER_ID",
		Short: "Permanently delete a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorUserClient()
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
			resp, err := client.DeleteUser(cmd.Context(), operatorRequest(&operatorv1.DeleteUserRequest{UserId: args[0]}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { fmt.Fprintf(out, "deleted user %s\n", args[0]) })
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm irreversible user deletion")
	return cmd
}

func operatorUserAddEmailCmd() *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "add-email USER_ID --email EMAIL",
		Short: "Add a verified email address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(email) == "" {
				return errors.New("--email is required")
			}
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.AddVerifiedEmail(cmd.Context(), operatorRequest(&operatorv1.AddVerifiedEmailRequest{
				UserId: args[0],
				Email:  email,
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { printOperatorUserLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address to add as already verified")
	return cmd
}

func operatorUserRoleCmd() *cobra.Command {
	roleCmd := &cobra.Command{
		Use:   "role",
		Short: "Manage user roles",
	}
	addCmd := &cobra.Command{
		Use:   "add USER_ID ROLE",
		Short: "Assign a role",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.AssignRole(cmd.Context(), operatorRequest(&operatorv1.AssignRoleRequest{
				UserId:   args[0],
				RoleName: args[1],
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { printOperatorUserLine(out, resp.Msg.GetMember()) })
		},
	}
	roleCmd.AddCommand(addCmd)

	removeCmd := &cobra.Command{
		Use:     "remove USER_ID ROLE",
		Aliases: []string{"rm"},
		Short:   "Revoke a role",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newOperatorUserClient()
			if err != nil {
				return err
			}
			resp, err := client.RevokeRole(cmd.Context(), operatorRequest(&operatorv1.RevokeRoleRequest{
				UserId:   args[0],
				RoleName: args[1],
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, resp.Msg, func() { printOperatorUserLine(out, resp.Msg.GetMember()) })
		},
	}
	roleCmd.AddCommand(removeCmd)
	return roleCmd
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

func printOperatorUserLine(out io.Writer, member *adminv1.AdminMember) {
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
