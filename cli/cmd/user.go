package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"hmans.de/chatto/internal/config"
	managementv1 "hmans.de/chatto/internal/pb/chatto/management/v1"
	"hmans.de/chatto/internal/pb/chatto/management/v1/managementv1connect"
)

var (
	userConfigFile       string
	userManagementSocket string
	userByID             bool

	userCreateLogin       string
	userCreateDisplayName string
	userCreateEmail       string
	userCreatePassword    string
	userCreatePrompt      bool

	userUpdateLogin       string
	userUpdateDisplayName string

	userPasswordValue  string
	userPasswordPrompt bool
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users through the private management socket",
}

var userCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a user through the running Chatto server",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := managementUserClient()
		if err != nil {
			return err
		}
		password, err := commandPassword(userCreatePassword, userCreatePrompt)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()

		resp, err := client.CreateUser(ctx, connect.NewRequest(&managementv1.CreateUserRequest{
			Login:         userCreateLogin,
			DisplayName:   userCreateDisplayName,
			Password:      password,
			VerifiedEmail: userCreateEmail,
		}))
		if err != nil {
			return err
		}
		printManagedUser("created", resp.Msg.GetUser())
		return nil
	},
}

var userUpdateCmd = &cobra.Command{
	Use:   "update USER",
	Short: "Update a user's login or display name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := managementUserClient()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()

		resp, err := client.UpdateUser(ctx, connect.NewRequest(&managementv1.UpdateUserRequest{
			User:        userSelector(args[0], userByID),
			Login:       userUpdateLogin,
			DisplayName: userUpdateDisplayName,
		}))
		if err != nil {
			return err
		}
		printManagedUser("updated", resp.Msg.GetUser())
		return nil
	},
}

var userPasswordCmd = &cobra.Command{
	Use:   "password USER",
	Short: "Set a user's password and revoke existing sessions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := managementUserClient()
		if err != nil {
			return err
		}
		password, err := commandPassword(userPasswordValue, userPasswordPrompt)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()

		resp, err := client.SetUserPassword(ctx, connect.NewRequest(&managementv1.SetUserPasswordRequest{
			User:     userSelector(args[0], userByID),
			Password: password,
		}))
		if err != nil {
			return err
		}
		printManagedUser("password updated", resp.Msg.GetUser())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.PersistentFlags().StringVarP(&userConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	userCmd.PersistentFlags().StringVar(&userManagementSocket, "management-socket", "", "path to the private management Unix socket")
	userCmd.PersistentFlags().BoolVar(&userByID, "by-id", false, "treat USER as a Chatto user ID instead of a login")

	userCmd.AddCommand(userCreateCmd)
	userCreateCmd.Flags().StringVar(&userCreateLogin, "login", "", "login for the new user")
	userCreateCmd.Flags().StringVar(&userCreateDisplayName, "display-name", "", "display name for the new user (default: login)")
	userCreateCmd.Flags().StringVar(&userCreateEmail, "email", "", "email address to add as already verified")
	userCreateCmd.Flags().StringVar(&userCreatePassword, "password", "", "initial password")
	userCreateCmd.Flags().BoolVar(&userCreatePrompt, "prompt-password", false, "prompt for the initial password")
	_ = userCreateCmd.MarkFlagRequired("login")

	userCmd.AddCommand(userUpdateCmd)
	userUpdateCmd.Flags().StringVar(&userUpdateLogin, "login", "", "new login")
	userUpdateCmd.Flags().StringVar(&userUpdateDisplayName, "display-name", "", "new display name")

	userCmd.AddCommand(userPasswordCmd)
	userPasswordCmd.Flags().StringVar(&userPasswordValue, "password", "", "new password")
	userPasswordCmd.Flags().BoolVar(&userPasswordPrompt, "prompt-password", true, "prompt for the new password")
}

func managementUserClient() (managementv1connect.UserAdminServiceClient, error) {
	socketPath, err := resolveManagementSocket(userManagementSocket, userConfigFile)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	httpClient := &http.Client{Transport: transport}
	return managementv1connect.NewUserAdminServiceClient(httpClient, "http://chatto-management"), nil
}

func resolveManagementSocket(flagValue, configPath string) (string, error) {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue), nil
	}
	if value := strings.TrimSpace(os.Getenv("CHATTO_MANAGEMENT_SOCKET")); value != "" {
		return value, nil
	}
	if value := strings.TrimSpace(os.Getenv("CHATTO_MANAGEMENT_SOCKET_PATH")); value != "" {
		return resolveManagementSocketRelativeToConfig(configPath, value)
	}

	path := configPath
	if path == "" {
		path = "chatto.toml"
	}
	configAbsPath, absErr := absoluteConfigPath(path)
	if absErr != nil {
		return "", absErr
	}
	if _, err := os.Stat(configAbsPath); err == nil {
		cfg, err := config.ReadConfig(configAbsPath)
		if err != nil {
			return "", err
		}
		if !cfg.Management.EnabledOrDefault() {
			return "", fmt.Errorf("management API is disabled in %s", configAbsPath)
		}
		return resolveManagementSocketRelativeToConfig(configAbsPath, cfg.Management.SocketPathOrDefault())
	} else if !os.IsNotExist(err) {
		return "", err
	}

	var cfg config.ManagementConfig
	return cfg.SocketPathOrDefault(), nil
}

func absoluteConfigPath(configPath string) (string, error) {
	if configPath == "" {
		configPath = "chatto.toml"
	}
	return filepath.Abs(configPath)
}

func resolveManagementSocketRelativeToConfig(configPath, socketPath string) (string, error) {
	if filepath.IsAbs(socketPath) {
		return socketPath, nil
	}
	configAbsPath, err := absoluteConfigPath(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configAbsPath), socketPath), nil
}

func commandPassword(value string, prompt bool) (string, error) {
	if value != "" {
		return value, nil
	}
	if !prompt {
		return "", nil
	}
	fmt.Fprint(os.Stderr, "Password: ")
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	password := string(bytes)
	if password == "" {
		return "", fmt.Errorf("password is required")
	}
	return password, nil
}

func userSelector(value string, byID bool) *managementv1.UserSelector {
	value = strings.TrimSpace(value)
	if byID {
		return &managementv1.UserSelector{
			Selector: &managementv1.UserSelector_UserId{UserId: value},
		}
	}
	return &managementv1.UserSelector{
		Selector: &managementv1.UserSelector_Login{Login: value},
	}
}

func printManagedUser(action string, user *managementv1.ManagedUser) {
	if user == nil {
		fmt.Printf("%s\n", action)
		return
	}
	fmt.Printf("%s user %s (%s, %s)\n", action, user.GetId(), user.GetLogin(), user.GetDisplayName())
}
