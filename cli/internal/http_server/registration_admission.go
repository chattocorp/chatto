package http_server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const registrationAdmissionsKey = "registration_admissions"

// registrationAdmission binds one tab's registration to a validated invitation.
// Only the random ID is exposed to JavaScript from the HttpOnly flow cookie. Four entries
// bound cookie size; expiry bounds admission even when the cookie lives longer.
type registrationAdmission struct {
	ID           string `json:"id"`
	InvitationID string `json:"invitation_id"`
	// CodeFingerprint invalidates remembered admission after root-secret rotation.
	CodeFingerprint string    `json:"code_fingerprint"`
	ExpiresAt       time.Time `json:"expires_at"`
}

func registrationAdmissions(session sessions.Session) []registrationAdmission {
	encoded, _ := session.Get(registrationAdmissionsKey).(string)
	var stored []registrationAdmission
	if json.Unmarshal([]byte(encoded), &stored) != nil {
		return nil
	}
	active := make([]registrationAdmission, 0, 4)
	for _, entry := range stored {
		if entry.ID != "" && time.Now().Before(entry.ExpiresAt) {
			active = append(active, entry)
		}
	}
	if len(active) > 4 {
		active = active[len(active)-4:]
	}
	return active
}

func saveRegistrationAdmissions(session sessions.Session, entries []registrationAdmission) error {
	if len(entries) == 0 {
		session.Delete(registrationAdmissionsKey)
	} else {
		encoded, err := json.Marshal(entries)
		if err != nil {
			return err
		}
		session.Set(registrationAdmissionsKey, string(encoded))
	}
	return session.Save()
}

func (s *HTTPServer) registrationInvitation(c *gin.Context, admissionID string) (string, error) {
	if admissionID == "" {
		return "", core.ErrInvitationInvalid
	}
	for _, entry := range registrationAdmissions(sessions.Default(c)) {
		if entry.ID == admissionID {
			if entry.CodeFingerprint != s.invitationFingerprint(entry.InvitationID) {
				return "", core.ErrInvitationInvalid
			}
			if err := s.core.ValidateInvitationAdmission(c.Request.Context(), entry.InvitationID); err != nil {
				return "", err
			}
			return entry.InvitationID, nil
		}
	}
	return "", core.ErrInvitationInvalid
}

func (s *HTTPServer) invitationFingerprint(id string) string {
	digest := sha256.Sum256([]byte(s.core.InvitationCode(id)))
	return base64.RawURLEncoding.EncodeToString(digest[:16])
}

// clearRegistrationAdmission removes only entries for the invitation consumed
// by this account creation. Other tabs can keep their own admission.
func clearRegistrationAdmission(c *gin.Context, invitationID string) {
	if invitationID == "" {
		return
	}
	session := sessions.Default(c)
	entries := registrationAdmissions(session)
	kept := make([]registrationAdmission, 0, len(entries))
	for _, entry := range entries {
		if entry.InvitationID != invitationID {
			kept = append(kept, entry)
		}
	}
	if len(kept) == len(entries) {
		return
	}
	if err := saveRegistrationAdmissions(session, kept); err != nil {
		log.Warn("Failed to clear completed registration admission")
	}
}

func (s *HTTPServer) setupRegistrationAdmissionRoutes(auth *gin.RouterGroup) {
	// Read-only status is never proof by itself. Account creation always checks
	// the bound durable invitation again, including across configuration changes.
	auth.GET("browser/registration-invitation", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !s.requestIsSameOrigin(c.Request) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		entries := registrationAdmissions(sessions.Default(c))
		for _, entry := range slices.Backward(entries) {
			if selected := c.Query("admission"); selected != "" && selected != entry.ID {
				continue
			}
			if _, err := s.registrationInvitation(c, entry.ID); err == nil {
				c.JSON(http.StatusOK, gin.H{"admissionId": entry.ID})
				return
			} else if !errors.Is(err, core.ErrInvitationInvalid) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Registration is temporarily unavailable"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"admissionId": ""})
	})
	auth.POST("browser/registration-invitation", s.requireBrowserAuthenticationRequest, s.requireServerSetupComplete, func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var request struct {
			Code                  string `json:"code" binding:"required,max=128"`
			ExternalIdentityToken string `json:"externalIdentityToken"`
		}
		if c.ShouldBindJSON(&request) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "An invitation code is required"})
			return
		}
		id, err := s.core.ValidateInvitationCode(c.Request.Context(), request.Code)
		if err != nil {
			if errors.Is(err, core.ErrInvitationInvalid) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "This invitation code is invalid or no longer available"})
			} else {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Registration is temporarily unavailable"})
			}
			return
		}
		pendingToken := ""
		if request.ExternalIdentityToken != "" {
			flow, err := s.core.GetPendingExternalIdentityCreateFlow(c.Request.Context(), request.ExternalIdentityToken)
			if err != nil {
				if errors.Is(err, core.ErrExternalIdentityFlowNotFound) || errors.Is(err, core.ErrExternalIdentityFlowExpired) || errors.Is(err, core.ErrExternalIdentityFlowWrongKind) {
					c.JSON(http.StatusGone, gin.H{"error": "This sign-up attempt has expired. Start again."})
				} else {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Registration is temporarily unavailable"})
				}
				return
			}
			flow.InvitationID = id
			// Copy the verified identity with its original expiry. Keep the
			// original capability so a lost response can be retried. Each copy
			// keeps its own invitation; account uniqueness prevents duplicate
			// accounts or consuming two codes for the same provider identity.
			pendingToken, err = s.core.CreatePendingExternalIdentityCreateFlow(c.Request.Context(), *flow)
			if err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Registration is temporarily unavailable"})
				return
			}
		}
		admissionID, err := randomString(24)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		session := sessions.Default(c)
		entries := registrationAdmissions(session)
		if len(entries) == 4 {
			entries = entries[1:]
		}
		entries = append(entries, registrationAdmission{ID: admissionID, InvitationID: id, CodeFingerprint: s.invitationFingerprint(id), ExpiresAt: time.Now().Add(30 * time.Minute)})
		if err := saveRegistrationAdmissions(session, entries); err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, gin.H{"admissionId": admissionID, "externalIdentityToken": pendingToken})
	})
}
