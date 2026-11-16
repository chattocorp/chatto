package config

// FrontendConfig contains settings for the bundled frontend that this server
// serves. The standalone frontend and the desktop app do not read them.
type FrontendConfig struct {
	Gutter GutterConfig `toml:"gutter,commented" comment:"Server Gutter in the bundled frontend."`
}

// GutterConfig controls the Server Gutter in the bundled frontend.
type GutterConfig struct {
	// Enabled shows the Server Gutter and the Server Directory, so users can
	// add and switch between other Chatto servers.
	Enabled bool `toml:"enabled" env:"CHATTO_FRONTEND_GUTTER_ENABLED" comment:"Show the Server Gutter and the Server Directory, so users can add and switch between other Chatto servers. When false, the bundled frontend shows only this server. Default: false."`
}
