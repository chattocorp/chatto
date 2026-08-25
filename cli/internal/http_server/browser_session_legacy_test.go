package http_server

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// saveCookieSession prepares the retired mixed-purpose cookie so focused tests
// can exercise the release-upgrade migration path.
func saveCookieSession(c *gin.Context, sessionID string) error {
	session := sessions.Default(c)
	session.Set(sessionKeyRuntimeCredentialID, sessionID)
	return session.Save()
}

func cookieCredentialIDFromSession(session sessions.Session) (string, bool) {
	if session == nil {
		return "", false
	}
	sessionID, _ := session.Get(sessionKeyRuntimeCredentialID).(string)
	return sessionID, sessionID != ""
}
