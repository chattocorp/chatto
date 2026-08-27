package config

import (
	"hmans.de/chatto/pkg/natsauth"
)

type EmbeddedNATSConfig struct {
	Enabled     bool   `toml:"enabled" env:"CHATTO_NATS_EMBEDDED_ENABLED" comment:"Enable embedded NATS server."`
	Port        int    `toml:"port,commented" env:"CHATTO_NATS_EMBEDDED_PORT" comment:"Uncomment to expose embedded NATS over TCP for nats CLI/admin commands. When left commented, Chatto connects in-process and no NATS port is opened."`
	BindAddress string `toml:"bind_address,commented" env:"CHATTO_NATS_EMBEDDED_BIND_ADDRESS" comment:"Address to bind NATS ports. Default: 127.0.0.1 (localhost only)."`
	HTTPPort    int    `toml:"http_port,commented" env:"CHATTO_NATS_EMBEDDED_HTTP_PORT" comment:"NATS monitoring/stats HTTP port. Set to 0 to disable."`
	DataDir     string `toml:"data_dir" env:"CHATTO_NATS_EMBEDDED_DATA_DIR" comment:"Directory where the embedded NATS server stores its data."`
	AuthToken   string `toml:"auth_token" env:"CHATTO_NATS_EMBEDDED_AUTH_TOKEN" comment:"Authentication token for NATS connections. Auto-generated on init."`
}

// BindAddressOrDefault returns the bind address, defaulting to localhost for security.
func (c *EmbeddedNATSConfig) BindAddressOrDefault() string {
	if c.BindAddress == "" {
		return "127.0.0.1"
	}
	return c.BindAddress
}

// NATSClientConfig contains settings for connecting to an external NATS server.
type NATSClientConfig struct {
	URL             string              `toml:"url" env:"CHATTO_NATS_CLIENT_URL" comment:"NATS server URL. Use a comma-separated list for cluster failover, e.g. nats://n1:4222,nats://n2:4222."`
	AuthMethod      natsauth.AuthMethod `toml:"auth_method" env:"CHATTO_NATS_CLIENT_AUTH_METHOD" comment:"Authentication method for the external NATS server: none, token, userpass, credentials, or nkey."`
	Token           string              `toml:"token" env:"CHATTO_NATS_CLIENT_TOKEN" comment:"Token for token auth. Only used when auth_method = 'token'. NEVER SHARE THIS!"`
	Username        string              `toml:"username,commented" env:"CHATTO_NATS_CLIENT_USERNAME" comment:"Username for userpass auth. Only used when auth_method = 'userpass'."`
	Password        string              `toml:"password,commented" env:"CHATTO_NATS_CLIENT_PASSWORD" comment:"Password for userpass auth. Only used when auth_method = 'userpass'. NEVER SHARE THIS!"`
	CredentialsFile string              `toml:"credentials_file,commented" env:"CHATTO_NATS_CLIENT_CREDENTIALS_FILE" comment:"Path to a NATS .creds file. Only used when auth_method = 'credentials'."`
	NKeySeed        string              `toml:"nkey_seed,commented" env:"CHATTO_NATS_CLIENT_NKEY_SEED" comment:"NKey seed. Only used when auth_method = 'nkey'. NEVER SHARE THIS!"`
	CACert          string              `toml:"ca_cert,commented" env:"CHATTO_NATS_CLIENT_CA_CERT" comment:"PEM-encoded CA certificate for verifying the NATS server's TLS certificate. When set, the connection uses TLS."`
}

// NATSAuthConfig returns the auth configuration suitable for natsauth.ConnectOptions.
func (c *NATSClientConfig) NATSAuthConfig() natsauth.Config {
	return natsauth.Config{
		AuthMethod:      c.AuthMethod,
		Token:           c.Token,
		Username:        c.Username,
		Password:        c.Password,
		CredentialsFile: c.CredentialsFile,
		NKeySeed:        c.NKeySeed,
		CACert:          c.CACert,
	}
}

type NATSConfig struct {
	Replicas int                `toml:"replicas" env:"CHATTO_NATS_REPLICAS" comment:"Number of replicas for JetStream streams, KV buckets, and object stores. Must be 1, 3, or 5 (odd numbers for quorum). Use 3 or 5 only with a matching NATS cluster."`
	Client   NATSClientConfig   `toml:"client,commented" comment:"External NATS client settings. To use an external server or cluster, set nats.embedded.enabled = false, then uncomment and update this section. Embedded NATS derives its client settings automatically."`
	Embedded EmbeddedNATSConfig `toml:"embedded"`
}

// ReplicasOrDefault returns the configured replicas count, defaulting to 1.
func (c *NATSConfig) ReplicasOrDefault() int {
	if c.Replicas <= 0 {
		return 1
	}
	return c.Replicas
}
