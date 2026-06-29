package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	"hmans.de/chatto/internal/pb/chatto/admin/v1/adminv1connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

var adminConfigFile string
var adminAPIURL string
var adminAPIToken string
var adminAPITokenFile string
var adminAPITokenName string
var adminOutputJSON bool

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Operator administration commands",
}

var adminUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users through the Admin API",
}

func init() {
	rootCmd.AddCommand(adminCmd)
	adminCmd.AddCommand(adminUserCmd)
	adminCmd.PersistentFlags().StringVarP(&adminConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	adminCmd.PersistentFlags().StringVar(&adminAPIURL, "url", "", "server URL or ConnectRPC base URL (default: webserver.url from config/env)")
	adminCmd.PersistentFlags().StringVar(&adminAPIToken, "admin-token", "", "Admin API bearer token; prefer --admin-token-file or CHATTO_ADMIN_API_TOKEN for automation")
	adminCmd.PersistentFlags().StringVar(&adminAPITokenFile, "admin-token-file", "", "file containing the Admin API bearer token")
	adminCmd.PersistentFlags().StringVar(&adminAPITokenName, "admin-token-name", "", "name of admin_api.tokens entry to use when reading token from config")
	adminCmd.PersistentFlags().BoolVar(&adminOutputJSON, "json", false, "print JSON output")

	adminUserCmd.AddCommand(
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
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAdminAPIClient()
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
			resp, err := client.ListMembers(cmd.Context(), adminRequest(&adminv1.ListMembersRequest{
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
	cmd.Flags().StringVar(&search, "search", "", "search login or display name")
	cmd.Flags().Int32Var(&limit, "limit", 20, "maximum users to return")
	cmd.Flags().Int32Var(&offset, "offset", 0, "zero-based result offset")
	return cmd
}

func adminUserGetCmd() *cobra.Command {
	var userID string
	var login string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a user by ID or login",
		RunE: func(cmd *cobra.Command, args []string) error {
			if (strings.TrimSpace(userID) == "") == (strings.TrimSpace(login) == "") {
				return errors.New("provide exactly one of --id or --login")
			}
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.GetMember(cmd.Context(), adminRequest(&adminv1.GetMemberRequest{UserId: userID, Login: login}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&userID, "id", "", "user ID")
	cmd.Flags().StringVar(&login, "login", "", "login")
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
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.CreateUser(cmd.Context(), adminRequest(&adminv1.CreateUserRequest{
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
	var login string
	var displayName string
	cmd := &cobra.Command{
		Use:   "update USER_ID",
		Short: "Update a user's profile fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &adminv1.UpdateUserRequest{UserId: args[0]}
			if cmd.Flags().Changed("login") {
				req.Login = &login
			}
			if cmd.Flags().Changed("display-name") {
				req.DisplayName = &displayName
			}
			if req.Login == nil && req.DisplayName == nil {
				return errors.New("provide --login and/or --display-name")
			}
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.UpdateUser(cmd.Context(), adminRequest(req))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
	cmd.Flags().StringVar(&login, "login", "", "new login")
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
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.SetUserPassword(cmd.Context(), adminRequest(&adminv1.SetUserPasswordRequest{
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
			if !yes {
				if !term.IsTerminal(int(syscall.Stdin)) {
					return errors.New("--yes is required when stdin is not a terminal")
				}
				if err := confirmDeletion(args[0]); err != nil {
					return err
				}
			}
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.DeleteUser(cmd.Context(), adminRequest(&adminv1.DeleteUserRequest{UserId: args[0]}))
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
	return &cobra.Command{
		Use:   "add-email USER_ID EMAIL",
		Short: "Add a verified email address",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.AddVerifiedEmail(cmd.Context(), adminRequest(&adminv1.AddVerifiedEmailRequest{
				UserId: args[0],
				Email:  args[1],
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	}
}

func adminUserRoleCmd() *cobra.Command {
	roleCmd := &cobra.Command{
		Use:   "role",
		Short: "Manage user roles",
	}
	roleCmd.AddCommand(&cobra.Command{
		Use:   "add USER_ID ROLE",
		Short: "Assign a role",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.AssignRole(cmd.Context(), adminRequest(&adminv1.AssignRoleRequest{
				UserId:   args[0],
				RoleName: args[1],
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	})
	roleCmd.AddCommand(&cobra.Command{
		Use:     "remove USER_ID ROLE",
		Aliases: []string{"rm"},
		Short:   "Revoke a role",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newAdminAPIClient()
			if err != nil {
				return err
			}
			resp, err := client.RevokeRole(cmd.Context(), adminRequest(&adminv1.RevokeRoleRequest{
				UserId:   args[0],
				RoleName: args[1],
			}))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printAdminOutput(out, resp.Msg, func() { printAdminMemberLine(out, resp.Msg.GetMember()) })
		},
	})
	return roleCmd
}

func newAdminAPIClient() (adminv1connect.AdminMemberServiceClient, error) {
	resolved, err := resolveAdminAPIClientConfig()
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: adminTokenTransport{
		token: resolved.token,
		base:  http.DefaultTransport,
	}}
	return adminv1connect.NewAdminMemberServiceClient(httpClient, resolved.connectBaseURL), nil
}

type resolvedAdminAPIConfig struct {
	connectBaseURL string
	token          string
}

func resolveAdminAPIClientConfig() (resolvedAdminAPIConfig, error) {
	envURL := strings.TrimSpace(os.Getenv("CHATTO_WEBSERVER_URL"))
	envToken := strings.TrimSpace(os.Getenv("CHATTO_ADMIN_API_TOKEN"))
	if err := validateSecretSources("--admin-token", adminAPIToken != "", "--admin-token-file", adminAPITokenFile != ""); err != nil {
		return resolvedAdminAPIConfig{}, err
	}
	resolved := resolvedAdminAPIConfig{
		connectBaseURL: strings.TrimSpace(adminAPIURL),
		token:          strings.TrimSpace(adminAPIToken),
	}
	if resolved.token == "" && adminAPITokenFile != "" {
		token, err := readTokenFile(adminAPITokenFile)
		if err != nil {
			return resolved, err
		}
		resolved.token = token
	}
	tokenName := strings.TrimSpace(adminAPITokenName)
	if envTokenName := strings.TrimSpace(os.Getenv("CHATTO_ADMIN_API_TOKEN_NAME")); tokenName == "" && envTokenName != "" {
		tokenName = envTokenName
	}
	explicitURL := resolved.connectBaseURL != ""
	if resolved.token == "" && envToken != "" {
		resolved.token = envToken
	}

	cfg, cfgErr := readAdminAPIConfigFile(adminConfigFile)
	if cfgErr != nil {
		return resolved, cfgErr
	}
	if err := applyAdminAPIEndpointEnv(&cfg); err != nil {
		return resolved, err
	}
	configuredURL := strings.TrimSpace(cfg.Webserver.URL)
	if envURL != "" {
		configuredURL = envURL
	}
	if cfg.AdminAPI.Enabled {
		configuredURL = cfg.AdminAPI.URLOrDefault()
	}
	if envTokens, envTokensSet, err := config.AdminAPITokensFromEnv(); err != nil {
		return resolved, err
	} else if envTokensSet {
		cfg.AdminAPI.Tokens = envTokens
	}
	if resolved.connectBaseURL == "" {
		if cfg.AdminAPI.Enabled {
			resolved.connectBaseURL = cfg.AdminAPI.URLOrDefault()
		} else if envURL != "" {
			resolved.connectBaseURL = envURL
		} else if cfg.Webserver.URL != "" {
			resolved.connectBaseURL = cfg.Webserver.URL
		}
	}
	if resolved.token == "" {
		if explicitURL && !adminAPIURLMatchesConfig(resolved.connectBaseURL, configuredURL) {
			return resolved, errors.New("refusing to send admin_api.tokens from config/env to an overridden admin API URL; set --admin-token or CHATTO_ADMIN_API_TOKEN")
		}
		token, err := selectAdminAPIConfigToken(cfg.AdminAPI.Tokens, tokenName)
		if err != nil {
			return resolved, err
		}
		resolved.token = token
	}
	if resolved.connectBaseURL == "" {
		return resolved, errors.New("admin API URL is required; set --url, CHATTO_WEBSERVER_URL, or webserver.url")
	}
	if resolved.token == "" {
		return resolved, errors.New("admin API token is required; set --admin-token, CHATTO_ADMIN_API_TOKEN, or admin_api.tokens")
	}
	baseURL, err := connectBaseURL(resolved.connectBaseURL)
	if err != nil {
		return resolved, err
	}
	resolved.connectBaseURL = baseURL
	return resolved, nil
}

func applyAdminAPIEndpointEnv(cfg *config.ChattoConfig) error {
	if v := strings.TrimSpace(os.Getenv("CHATTO_ADMIN_API_ENABLED")); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid CHATTO_ADMIN_API_ENABLED: %w", err)
		}
		cfg.AdminAPI.Enabled = enabled
	}
	if v := strings.TrimSpace(os.Getenv("CHATTO_ADMIN_API_BIND_ADDRESS")); v != "" {
		cfg.AdminAPI.BindAddress = v
	}
	if v := strings.TrimSpace(os.Getenv("CHATTO_ADMIN_API_PORT")); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("invalid CHATTO_ADMIN_API_PORT: %w", err)
		}
		cfg.AdminAPI.Port = port
	}
	return nil
}

func adminAPIURLMatchesConfig(rawURL, rawConfigURL string) bool {
	if strings.TrimSpace(rawURL) == "" || strings.TrimSpace(rawConfigURL) == "" {
		return false
	}
	urlBase, err := connectBaseURL(rawURL)
	if err != nil {
		return false
	}
	configBase, err := connectBaseURL(rawConfigURL)
	if err != nil {
		return false
	}
	return urlBase == configBase
}

func selectAdminAPIConfigToken(tokens []config.AdminAPITokenConfig, name string) (string, error) {
	if len(tokens) == 0 {
		return "", nil
	}
	if name == "" {
		return strings.TrimSpace(tokens[0].Token), nil
	}
	for _, token := range tokens {
		if token.Name == name {
			return strings.TrimSpace(token.Token), nil
		}
	}
	return "", fmt.Errorf("admin API token named %q not found in admin_api.tokens", name)
}

func readAdminAPIConfigFile(path string) (config.ChattoConfig, error) {
	var cfg config.ChattoConfig
	if path == "" {
		path = "chatto.toml"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && adminConfigFile == "" {
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

func readTokenFile(path string) (string, error) {
	token, err := readSecretFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
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

func connectBaseURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("admin API URL must be absolute: %s", raw)
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname())) {
		return "", fmt.Errorf("admin API URL must use https unless it targets a loopback host: %s", raw)
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(u.Path, connectapi.Prefix) {
		u.Path = strings.TrimRight(u.Path, "/") + connectapi.Prefix
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type adminTokenTransport struct {
	token string
	base  http.RoundTripper
}

func (t adminTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	cloned.Header.Set("Authorization", "Bearer "+t.token)
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

func adminRequest[T any](msg *T) *connect.Request[T] {
	return connect.NewRequest(msg)
}

func printAdminOutput(out io.Writer, message proto.Message, human func()) error {
	if adminOutputJSON {
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
