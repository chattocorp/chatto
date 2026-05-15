// Package natsauth provides authentication option builders for NATS connections.
// It supports token, username/password, credentials file, and NKey authentication,
// plus optional TLS with a custom CA.
package natsauth

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

// AuthMethod defines how to authenticate with NATS.
type AuthMethod string

const (
	AuthNone        AuthMethod = "none"        // No authentication
	AuthToken       AuthMethod = "token"       // Simple bearer token
	AuthUserPass    AuthMethod = "userpass"    // Username/password
	AuthCredentials AuthMethod = "credentials" // JWT credentials file
	AuthNKey        AuthMethod = "nkey"        // NKey seed
)

// Config holds the parameters needed to build NATS authentication + TLS options.
type Config struct {
	AuthMethod      AuthMethod
	Token           string
	Username        string
	Password        string
	CredentialsFile string
	NKeySeed        string

	// CACert is a PEM-encoded CA certificate used to verify the NATS server's
	// TLS certificate. When non-empty, a nats.Secure option is added to the
	// connection. When empty, no TLS option is added — the connection uses
	// system defaults (which kick in automatically if the URL is tls://).
	CACert string
}

// ConnectOptions returns NATS connection options for the given auth + TLS configuration.
func ConnectOptions(cfg Config) ([]nats.Option, error) {
	opts, err := authOptions(cfg)
	if err != nil {
		return nil, err
	}

	if cfg.CACert != "" {
		tlsOpt, err := tlsOption(cfg.CACert)
		if err != nil {
			return nil, err
		}
		opts = append(opts, tlsOpt)
	}

	return opts, nil
}

// authOptions returns the auth-method-specific connection options.
func authOptions(cfg Config) ([]nats.Option, error) {
	switch cfg.AuthMethod {
	case AuthNone, "":
		return nil, nil

	case AuthToken:
		if cfg.Token == "" {
			return nil, fmt.Errorf("nats auth: token is required for token method")
		}
		return []nats.Option{nats.Token(cfg.Token)}, nil

	case AuthUserPass:
		if cfg.Username == "" {
			return nil, fmt.Errorf("nats auth: username is required for userpass method")
		}
		return []nats.Option{nats.UserInfo(cfg.Username, cfg.Password)}, nil

	case AuthCredentials:
		if cfg.CredentialsFile == "" {
			return nil, fmt.Errorf("nats auth: credentials_file is required for credentials method")
		}
		return []nats.Option{nats.UserCredentials(cfg.CredentialsFile)}, nil

	case AuthNKey:
		if cfg.NKeySeed == "" {
			return nil, fmt.Errorf("nats auth: nkey_seed is required for nkey method")
		}
		opt, err := nkeyOption(cfg.NKeySeed)
		if err != nil {
			return nil, err
		}
		return []nats.Option{opt}, nil

	default:
		return nil, fmt.Errorf("nats auth: unknown method %q", cfg.AuthMethod)
	}
}

// tlsOption builds a nats.Secure option from a PEM-encoded CA certificate.
func tlsOption(caPEM string) (nats.Option, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, fmt.Errorf("nats auth: parsing CA cert PEM")
	}
	return nats.Secure(&tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
	}), nil
}

// nkeyOption creates a NATS option for NKey authentication.
func nkeyOption(seed string) (nats.Option, error) {
	kp, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return nil, fmt.Errorf("nats auth: invalid nkey seed: %w", err)
	}

	pubKey, err := kp.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("nats auth: failed to get public key: %w", err)
	}

	return nats.Nkey(pubKey, func(nonce []byte) ([]byte, error) {
		return kp.Sign(nonce)
	}), nil
}

// PublicKeyFromSeed extracts the public key from an NKey seed.
// This is useful for generating the nats-server.conf authorization section.
func PublicKeyFromSeed(seed string) (string, error) {
	kp, err := nkeys.FromSeed([]byte(seed))
	if err != nil {
		return "", fmt.Errorf("invalid nkey seed: %w", err)
	}
	return kp.PublicKey()
}

// GenerateUserNKey generates a new user NKey pair.
// Returns the seed (private, for config) and public key (for nats-server.conf).
func GenerateUserNKey() (seed, publicKey string, err error) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		return "", "", fmt.Errorf("failed to create user nkey: %w", err)
	}

	seedBytes, err := kp.Seed()
	if err != nil {
		return "", "", fmt.Errorf("failed to get seed: %w", err)
	}

	pubKey, err := kp.PublicKey()
	if err != nil {
		return "", "", fmt.Errorf("failed to get public key: %w", err)
	}

	return string(seedBytes), pubKey, nil
}
