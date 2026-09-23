package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"connectrpc.com/connect"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/connectapi"
)

var operatorConfigFile string
var operatorSocketPath string
var operatorOutputJSON bool

var operatorCmd = &cobra.Command{
	Use:   "operator",
	Short: "Local operator commands",
}

func init() {
	rootCmd.AddCommand(operatorCmd)
	operatorCmd.PersistentFlags().StringVarP(&operatorConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	operatorCmd.PersistentFlags().StringVar(&operatorSocketPath, "operator-socket", "", "operator API Unix socket path")
	operatorCmd.PersistentFlags().BoolVar(&operatorOutputJSON, "json", false, "print JSON output")
}

// newOperatorHTTPClient connects operator services through the local Unix socket.
// The returned base URL is used only for ConnectRPC routing; it is not a network destination.
func newOperatorHTTPClient() (*http.Client, string, error) {
	resolved, err := resolveOperatorAPIClientConfig()
	if err != nil {
		return nil, "", err
	}
	return &http.Client{Transport: newOperatorSocketTransport(resolved.socketPath)}, resolved.connectBaseURL, nil
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

func newOperatorSocketTransport(socketPath string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
}

func operatorRequest[T any](msg *T) *connect.Request[T] {
	return connect.NewRequest(msg)
}

func printOperatorOutput(out io.Writer, message proto.Message, human func()) error {
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
