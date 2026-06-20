package http_server

import (
	"errors"
	"net/http"

	"github.com/charmbracelet/log"
	ginsessions "github.com/gin-contrib/sessions"
	"github.com/gorilla/securecookie"
	gsessions "github.com/gorilla/sessions"
)

type debugSessionStore struct {
	ginsessions.Store
	logger *log.Logger
}

func newDebugSessionStore(store ginsessions.Store, logger *log.Logger) ginsessions.Store {
	return &debugSessionStore{
		Store:  store,
		logger: logger,
	}
}

func (s *debugSessionStore) Get(r *http.Request, name string) (*gsessions.Session, error) {
	session, err := s.Store.Get(r, name)
	if err == nil {
		return session, nil
	}

	if isExpectedSessionCookieDecodeError(err) && session != nil {
		s.logger.Debug("Ignoring invalid session cookie",
			"cookieName", name,
			"hasCookie", hasCookieNamed(r, name),
			"reason", err.Error(),
		)
		return session, nil
	}

	return session, err
}

func isExpectedSessionCookieDecodeError(err error) bool {
	var secureCookieErr securecookie.Error
	return errors.As(err, &secureCookieErr) &&
		secureCookieErr.IsDecode() &&
		!secureCookieErr.IsUsage() &&
		!secureCookieErr.IsInternal()
}

func hasCookieNamed(r *http.Request, name string) bool {
	_, err := r.Cookie(name)
	return err == nil
}
