package config

// BootstrapConfig declares users and the server config to be auto-applied
// on startup, for fast iteration while developing and for E2E test fixtures.
// ONLY honored by builds compiled with the `bootstrap` build tag — release
// binaries parse the section but ignore its contents. Plaintext passwords
// are fine here for the same reason.
type BootstrapConfig struct {
	Users          []BootstrapUser  `toml:"users"`
	Server         *BootstrapServer `toml:"server,commented" comment:"Seeds the server config (name) and the deployment's primary room group on first boot."`
	LegacyInstance *BootstrapServer `toml:"instance,commented" comment:"Deprecated alias for [bootstrap.server]. Prefer [bootstrap.server]."`
}

// BootstrapUser describes a user to create on startup in bootstrap-tag builds.
type BootstrapUser struct {
	Login        string `toml:"login" comment:"Required. The user's login (username)."`
	DisplayName  string `toml:"display_name,commented" comment:"Defaults to Login if empty."`
	Email        string `toml:"email,commented" comment:"Optional. If set, added as a verified email."`
	Password     string `toml:"password,commented" comment:"Optional. Required to log in via password; safe in plaintext because bootstrap-tag builds only."`
	ServerRole   string `toml:"server_role,commented" comment:"Optional: owner | admin | moderator."`
	InstanceRole string `toml:"instance_role,commented" comment:"Deprecated alias for server_role. Prefer server_role."`
}

// RoleOrDefault returns the normalized bootstrap role, honoring the deprecated
// instance_role alias only when server_role is unset.
func (u BootstrapUser) RoleOrDefault() string {
	if u.ServerRole != "" {
		return u.ServerRole
	}
	return u.InstanceRole
}

// ServerOrDefault returns the normalized bootstrap server, honoring the
// deprecated [bootstrap.instance] alias only when [bootstrap.server] is unset.
func (c BootstrapConfig) ServerOrDefault() *BootstrapServer {
	if c.Server != nil {
		return c.Server
	}
	return c.LegacyInstance
}

// BootstrapServer describes the server to seed on startup in bootstrap-tag
// builds. Per ADR-027 there is no separate "space" concept any more — the
// server is the product surface. The bootstrap creates whatever underlying storage
// records (notably a primary space) the data layer still needs, but those
// are internal: operators only configure the server's name.
type BootstrapServer struct {
	Name  string   `toml:"name" comment:"Required. The instance's display name."`
	Rooms []string `toml:"rooms,commented" comment:"Optional. Auto-join rooms created on the instance; defaults to announcements + general."`
}
