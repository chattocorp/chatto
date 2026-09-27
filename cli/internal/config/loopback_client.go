package config

import "time"

const (
	// ChattoLoopbackClientID identifies the bundled browser client when it runs
	// on a loopback origin, such as a local development stack, and signs in to
	// a server that is not local. That server cannot retrieve a CIMD document
	// from the user's loopback interface, so this built-in identity replaces the
	// origin's frontend CIMD URL. Any local process can use this identity, so it
	// does not endorse the client. Servers accept it only when
	// auth.loopback_client_enabled is set.
	ChattoLoopbackClientID = "chatto://loopback"

	// ChattoLoopbackClientName is the non-endorsing display name recorded for
	// the loopback client in the administrator OAuth client inventory.
	ChattoLoopbackClientName = "Unverified local app"

	// ChattoLoopbackOAuthCallback is the popup callback accepted for the loopback
	// client. Its scheme, host, and port are placeholders: the client accepts the
	// same path and query on any HTTP or HTTPS loopback origin.
	ChattoLoopbackOAuthCallback = "http://localhost/servers/callback?mode=popup"

	// ChattoLoopbackSessionLifetime is the fixed lifetime of a renewable session
	// issued to the loopback client. Refresh does not extend it, which limits
	// the impact of a code that another local process intercepted.
	ChattoLoopbackSessionLifetime = 24 * time.Hour
)

// IsBuiltInOAuthClientID reports whether clientID names an OAuth client that
// Chatto registers itself instead of resolving through CIMD.
func IsBuiltInOAuthClientID(clientID string) bool {
	switch clientID {
	case ChattoDesktopOrigin, ChattoMobileClientID, ChattoLoopbackClientID:
		return true
	}
	return false
}
