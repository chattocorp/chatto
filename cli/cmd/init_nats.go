package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/nats-io/nkeys"
	"github.com/pelletier/go-toml/v2"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsauth"
)

func validateInitNATSURL(value string) error {
	for raw := range strings.SplitSeq(value, ",") {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return errors.New("enter NATS server URLs without credentials, paths, or query parameters")
		}
		switch u.Scheme {
		case "nats", "tls", "ws", "wss":
		default:
			return errors.New("use nats://, tls://, ws://, or wss:// for the NATS server")
		}
		if u.Port() != "" {
			if err := validateInitPort(u.Port()); err != nil {
				return fmt.Errorf("NATS port: %w", err)
			}
		}
	}
	return nil
}

func validateInitNATSClient(client config.NATSClientConfig) error {
	if err := validateInitNATSURL(client.URL); err != nil {
		return err
	}
	switch client.AuthMethod {
	case natsauth.AuthNone:
		return nil
	case natsauth.AuthToken:
		if strings.TrimSpace(client.Token) == "" {
			return errors.New("enter the NATS token")
		}
	case natsauth.AuthUserPass:
		if strings.TrimSpace(client.Username) == "" {
			return errors.New("enter the NATS username")
		}
	case natsauth.AuthCredentials:
		if strings.TrimSpace(client.CredentialsFile) == "" {
			return errors.New("enter the NATS credentials file path")
		}
	case natsauth.AuthNKey:
		pair, err := nkeys.FromSeed([]byte(client.NKeySeed))
		if err != nil {
			return errors.New("enter a valid NATS NKey seed")
		}
		pair.Wipe()
	default:
		return errors.New("choose a NATS authentication method")
	}
	return nil
}

// marshalInitExternalNATS activates the selected client's settings. The normal
// config type comments out external authentication fields for embedded setups.
// Only credentials for the selected method are written, even after backtracking.
func marshalInitExternalNATS(client config.NATSClientConfig) ([]byte, error) {
	type activeClient struct {
		URL             string              `toml:"url"`
		AuthMethod      natsauth.AuthMethod `toml:"auth_method"`
		Token           string              `toml:"token,omitempty"`
		Username        string              `toml:"username,omitempty"`
		Password        string              `toml:"password,omitempty"`
		CredentialsFile string              `toml:"credentials_file,omitempty"`
		NKeySeed        string              `toml:"nkey_seed,omitempty"`
	}
	selected := activeClient{URL: client.URL, AuthMethod: client.AuthMethod}
	switch client.AuthMethod {
	case natsauth.AuthToken:
		selected.Token = client.Token
	case natsauth.AuthUserPass:
		selected.Username, selected.Password = client.Username, client.Password
	case natsauth.AuthCredentials:
		selected.CredentialsFile = client.CredentialsFile
	case natsauth.AuthNKey:
		selected.NKeySeed = client.NKeySeed
	}
	type activeNATS struct {
		Replicas int          `toml:"replicas" comment:"Number of JetStream replicas. Use 3 or 5 only with a matching NATS cluster."`
		Client   activeClient `toml:"client" comment:"External NATS server with JetStream enabled."`
		Embedded struct {
			Enabled bool `toml:"enabled"`
		} `toml:"embedded"`
	}
	return toml.Marshal(struct {
		NATS activeNATS `toml:"nats"`
	}{NATS: activeNATS{Replicas: 1, Client: selected}})
}
