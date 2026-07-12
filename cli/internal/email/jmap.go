package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"hmans.de/chatto/internal/config"
)

const (
	jmapCoreCapability       = "urn:ietf:params:jmap:core"
	jmapMailCapability       = "urn:ietf:params:jmap:mail"
	jmapSubmissionCapability = "urn:ietf:params:jmap:submission"
	jmapRequestTimeout       = 15 * time.Second
	jmapResponseMaxBytes     = 4 << 20
)

// JMAPMailer sends transactional email through the JMAP submission API. It
// creates a plaintext draft, submits it, and removes the draft after a
// successful submission.
type JMAPMailer struct {
	config config.JMAPConfig
	client *http.Client
}

// Verify JMAPMailer implements Sender at compile time.
var _ Sender = (*JMAPMailer)(nil)

// NewJMAPMailer creates a JMAP transactional email sender.
func NewJMAPMailer(cfg config.JMAPConfig) *JMAPMailer {
	return newJMAPMailer(cfg, &http.Client{
		Timeout: jmapRequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

func newJMAPMailer(cfg config.JMAPConfig, client *http.Client) *JMAPMailer {
	return &JMAPMailer{config: cfg, client: client}
}

// Send sends an email through JMAP.
func (m *JMAPMailer) Send(msg Message) error {
	return m.SendContext(context.Background(), msg)
}

// SendContext sends an email through JMAP with context support. A successful
// response means the JMAP server accepted the submission; it does not confirm
// final recipient delivery.
func (m *JMAPMailer) SendContext(ctx context.Context, msg Message) error {
	if !m.IsEnabled() {
		return ErrEmailDisabled
	}

	from, err := mail.ParseAddress(m.config.From)
	if err != nil {
		return fmt.Errorf("invalid JMAP from address: %w", err)
	}
	to, err := mail.ParseAddress(msg.To)
	if err != nil {
		return fmt.Errorf("invalid JMAP recipient address: %w", err)
	}

	session, err := m.getSession(ctx)
	if err != nil {
		return err
	}
	accountID, err := m.selectAccount(session)
	if err != nil {
		return err
	}
	identityID, draftMailboxID, err := m.resolveSubmissionResources(ctx, session.APIURL, accountID, from.Address)
	if err != nil {
		return err
	}

	request := jmapRequest{
		Using: []string{jmapCoreCapability, jmapMailCapability, jmapSubmissionCapability},
		MethodCalls: []jmapMethodCall{
			{
				Name: "Email/set",
				Arguments: jmapEmailSetRequest{
					AccountID: accountID,
					Create: map[string]jmapEmail{
						"email": {
							MailboxIDs: map[string]bool{draftMailboxID: true},
							Keywords:   map[string]bool{"$draft": true},
							From:       []jmapAddress{{Name: from.Name, Email: from.Address}},
							To:         []jmapAddress{{Name: to.Name, Email: to.Address}},
							Subject:    msg.Subject,
							BodyStructure: jmapBodyPart{
								PartID: "text",
								Type:   "text/plain",
							},
							BodyValues: map[string]jmapBodyValue{
								"text": {Value: msg.Body},
							},
						},
					},
				},
				CallID: "create-email",
			},
			{
				Name: "EmailSubmission/set",
				Arguments: jmapSubmissionSetRequest{
					AccountID: accountID,
					Create: map[string]jmapSubmission{
						"submission": {IdentityID: identityID, EmailID: "#email"},
					},
					OnSuccessDestroyEmail: []string{"#submission"},
				},
				CallID: "submit-email",
			},
		},
	}

	response, err := m.call(ctx, session.APIURL, request)
	if err != nil {
		return err
	}
	emailID, err := response.createdID("Email/set", "create-email", "email")
	if err != nil {
		return err
	}
	if _, err := response.createdID("EmailSubmission/set", "submit-email", "submission"); err != nil {
		return err
	}
	if err := response.requireDestroyedEmail("submit-email", emailID); err != nil {
		return err
	}

	return nil
}

// IsEnabled reports whether the JMAP sender has its required credentials and
// sender address. Startup validation provides the user-facing configuration
// errors; this guard keeps direct construction safe for tests and callers.
func (m *JMAPMailer) IsEnabled() bool {
	return strings.TrimSpace(m.config.SessionURL) != "" &&
		strings.TrimSpace(m.config.AccessToken) != "" &&
		strings.TrimSpace(m.config.From) != ""
}

func (m *JMAPMailer) getSession(ctx context.Context) (jmapSession, error) {
	if err := validateJMAPHTTPSURL("JMAP session URL", m.config.SessionURL); err != nil {
		return jmapSession{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.config.SessionURL, nil)
	if err != nil {
		return jmapSession{}, fmt.Errorf("create JMAP session request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.config.AccessToken)

	response, err := m.client.Do(req)
	if err != nil {
		return jmapSession{}, fmt.Errorf("request JMAP session: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return jmapSession{}, fmt.Errorf("JMAP session request returned HTTP %d", response.StatusCode)
	}

	var session jmapSession
	if err := json.NewDecoder(io.LimitReader(response.Body, jmapResponseMaxBytes)).Decode(&session); err != nil {
		return jmapSession{}, fmt.Errorf("decode JMAP session response: %w", err)
	}
	if session.APIURL == "" {
		return jmapSession{}, fmt.Errorf("JMAP session response has no API URL")
	}
	if err := validateJMAPHTTPSURL("JMAP session API URL", session.APIURL); err != nil {
		return jmapSession{}, err
	}
	if !session.hasCapability(jmapCoreCapability) || !session.hasCapability(jmapMailCapability) || !session.hasCapability(jmapSubmissionCapability) {
		return jmapSession{}, fmt.Errorf("JMAP server does not support mail submission")
	}
	return session, nil
}

func (m *JMAPMailer) selectAccount(session jmapSession) (string, error) {
	accountID := m.config.AccountID
	if accountID == "" {
		accountID = session.PrimaryAccounts[jmapSubmissionCapability]
	}
	if accountID == "" {
		return "", fmt.Errorf("JMAP session has no primary submission account; set email.jmap.account_id")
	}
	account, ok := session.Accounts[accountID]
	if !ok {
		return "", fmt.Errorf("configured JMAP account is not available")
	}
	if !account.hasCapability(jmapMailCapability) || !account.hasCapability(jmapSubmissionCapability) {
		return "", fmt.Errorf("configured JMAP account does not support mail submission")
	}
	return accountID, nil
}

func (m *JMAPMailer) resolveSubmissionResources(ctx context.Context, apiURL, accountID, fromAddress string) (string, string, error) {
	request := jmapRequest{
		Using: []string{jmapCoreCapability, jmapMailCapability, jmapSubmissionCapability},
		MethodCalls: []jmapMethodCall{
			{Name: "Identity/get", Arguments: jmapGetRequest{AccountID: accountID}, CallID: "identities"},
			{Name: "Mailbox/get", Arguments: jmapGetRequest{AccountID: accountID}, CallID: "mailboxes"},
		},
	}
	response, err := m.call(ctx, apiURL, request)
	if err != nil {
		return "", "", err
	}

	identities, err := response.identities("identities")
	if err != nil {
		return "", "", err
	}
	mailboxes, err := response.mailboxes("mailboxes")
	if err != nil {
		return "", "", err
	}

	identityID, err := m.selectIdentity(identities, fromAddress)
	if err != nil {
		return "", "", err
	}
	draftMailboxID, err := m.selectDraftMailbox(mailboxes)
	if err != nil {
		return "", "", err
	}
	return identityID, draftMailboxID, nil
}

func (m *JMAPMailer) selectIdentity(identities []jmapIdentity, fromAddress string) (string, error) {
	if m.config.IdentityID != "" {
		for _, identity := range identities {
			if identity.ID == m.config.IdentityID {
				return identity.ID, nil
			}
		}
		return "", fmt.Errorf("configured JMAP identity is not available")
	}
	for _, identity := range identities {
		if strings.EqualFold(identity.Email, fromAddress) {
			return identity.ID, nil
		}
	}
	for _, identity := range identities {
		if jmapIdentityMatches(identity.Email, fromAddress) {
			return identity.ID, nil
		}
	}
	return "", fmt.Errorf("JMAP account has no identity matching email.jmap.from; set email.jmap.identity_id")
}

func jmapIdentityMatches(identityAddress, fromAddress string) bool {
	if !strings.HasPrefix(identityAddress, "*@") {
		return false
	}
	_, fromDomain, found := strings.Cut(fromAddress, "@")
	return found && strings.EqualFold(strings.TrimPrefix(identityAddress, "*@"), fromDomain)
}

func (m *JMAPMailer) selectDraftMailbox(mailboxes []jmapMailbox) (string, error) {
	if m.config.DraftMailboxID != "" {
		for _, mailbox := range mailboxes {
			if mailbox.ID == m.config.DraftMailboxID {
				return mailbox.ID, nil
			}
		}
		return "", fmt.Errorf("configured JMAP Drafts mailbox is not available")
	}
	for _, mailbox := range mailboxes {
		if mailbox.Role == "drafts" {
			return mailbox.ID, nil
		}
	}
	return "", fmt.Errorf("JMAP account has no Drafts mailbox; set email.jmap.draft_mailbox_id")
}

func (m *JMAPMailer) call(ctx context.Context, apiURL string, payload jmapRequest) (jmapResponse, error) {
	if err := validateJMAPHTTPSURL("JMAP API URL", apiURL); err != nil {
		return jmapResponse{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return jmapResponse{}, fmt.Errorf("encode JMAP request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return jmapResponse{}, fmt.Errorf("create JMAP API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.config.AccessToken)

	response, err := m.client.Do(req)
	if err != nil {
		return jmapResponse{}, fmt.Errorf("request JMAP API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return jmapResponse{}, fmt.Errorf("JMAP API request returned HTTP %d", response.StatusCode)
	}

	var decoded jmapResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, jmapResponseMaxBytes)).Decode(&decoded); err != nil {
		return jmapResponse{}, fmt.Errorf("decode JMAP API response: %w", err)
	}
	return decoded, nil
}

func validateJMAPHTTPSURL(name, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%s must be an absolute HTTPS URL", name)
	}
	return nil
}

type jmapSession struct {
	APIURL          string                     `json:"apiUrl"`
	Capabilities    map[string]json.RawMessage `json:"capabilities"`
	Accounts        map[string]jmapAccount     `json:"accounts"`
	PrimaryAccounts map[string]string          `json:"primaryAccounts"`
}

func (s jmapSession) hasCapability(capability string) bool {
	_, ok := s.Capabilities[capability]
	return ok
}

type jmapAccount struct {
	AccountCapabilities map[string]json.RawMessage `json:"accountCapabilities"`
}

func (a jmapAccount) hasCapability(capability string) bool {
	_, ok := a.AccountCapabilities[capability]
	return ok
}

type jmapRequest struct {
	Using       []string         `json:"using"`
	MethodCalls []jmapMethodCall `json:"methodCalls"`
}

type jmapMethodCall struct {
	Name      string
	Arguments any
	CallID    string
}

func (c jmapMethodCall) MarshalJSON() ([]byte, error) {
	return json.Marshal([3]any{c.Name, c.Arguments, c.CallID})
}

type jmapGetRequest struct {
	AccountID string `json:"accountId"`
}

type jmapIdentity struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type jmapMailbox struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type jmapAddress struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

type jmapBodyPart struct {
	PartID string `json:"partId"`
	Type   string `json:"type"`
}

type jmapBodyValue struct {
	Value string `json:"value"`
}

type jmapEmail struct {
	MailboxIDs    map[string]bool          `json:"mailboxIds"`
	Keywords      map[string]bool          `json:"keywords"`
	From          []jmapAddress            `json:"from"`
	To            []jmapAddress            `json:"to"`
	Subject       string                   `json:"subject"`
	BodyStructure jmapBodyPart             `json:"bodyStructure"`
	BodyValues    map[string]jmapBodyValue `json:"bodyValues"`
}

type jmapEmailSetRequest struct {
	AccountID string               `json:"accountId"`
	Create    map[string]jmapEmail `json:"create"`
}

type jmapSubmission struct {
	IdentityID string `json:"identityId"`
	EmailID    string `json:"emailId"`
}

type jmapSubmissionSetRequest struct {
	AccountID             string                    `json:"accountId"`
	Create                map[string]jmapSubmission `json:"create"`
	OnSuccessDestroyEmail []string                  `json:"onSuccessDestroyEmail"`
}

type jmapResponse struct {
	MethodResponses [][]json.RawMessage `json:"methodResponses"`
}

func (r jmapResponse) find(callID, expectedMethod string) ([]json.RawMessage, error) {
	for _, response := range r.MethodResponses {
		if len(response) != 3 {
			continue
		}
		responseCallID, err := jmapResponseString(response[2])
		if err != nil || responseCallID != callID {
			continue
		}
		methodName, err := jmapResponseString(response[0])
		if err != nil {
			return nil, fmt.Errorf("decode JMAP response method")
		}
		if methodName == "error" {
			var errorResponse struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(response[1], &errorResponse); err != nil {
				return nil, fmt.Errorf("decode JMAP error response")
			}
			if errorResponse.Type == "" {
				return nil, fmt.Errorf("JMAP method %q failed", callID)
			}
			return nil, fmt.Errorf("JMAP method %q failed: %s", callID, errorResponse.Type)
		}
		if methodName == expectedMethod {
			return response, nil
		}
	}
	return nil, fmt.Errorf("JMAP response did not include method %q for call %q", expectedMethod, callID)
}

func jmapResponseString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func (r jmapResponse) identities(callID string) ([]jmapIdentity, error) {
	response, err := r.find(callID, "Identity/get")
	if err != nil {
		return nil, err
	}
	methodName, err := jmapResponseString(response[0])
	if err != nil || methodName != "Identity/get" {
		return nil, fmt.Errorf("unexpected JMAP response for method %q", callID)
	}
	var payload struct {
		List []jmapIdentity `json:"list"`
	}
	if err := json.Unmarshal(response[1], &payload); err != nil {
		return nil, fmt.Errorf("decode JMAP identities response: %w", err)
	}
	return payload.List, nil
}

func (r jmapResponse) mailboxes(callID string) ([]jmapMailbox, error) {
	response, err := r.find(callID, "Mailbox/get")
	if err != nil {
		return nil, err
	}
	methodName, err := jmapResponseString(response[0])
	if err != nil || methodName != "Mailbox/get" {
		return nil, fmt.Errorf("unexpected JMAP response for method %q", callID)
	}
	var payload struct {
		List []jmapMailbox `json:"list"`
	}
	if err := json.Unmarshal(response[1], &payload); err != nil {
		return nil, fmt.Errorf("decode JMAP mailboxes response: %w", err)
	}
	return payload.List, nil
}

func (r jmapResponse) createdID(methodName, callID, creationID string) (string, error) {
	response, err := r.find(callID, methodName)
	if err != nil {
		return "", err
	}
	returnedMethodName, err := jmapResponseString(response[0])
	if err != nil || returnedMethodName != methodName {
		return "", fmt.Errorf("unexpected JMAP response for method %q", callID)
	}
	var payload struct {
		Created map[string]struct {
			ID string `json:"id"`
		} `json:"created"`
		NotCreated map[string]struct {
			Type string `json:"type"`
		} `json:"notCreated"`
	}
	if err := json.Unmarshal(response[1], &payload); err != nil {
		return "", fmt.Errorf("decode JMAP response for method %q: %w", callID, err)
	}
	if created, ok := payload.Created[creationID]; ok && created.ID != "" {
		return created.ID, nil
	}
	if failure, ok := payload.NotCreated[creationID]; ok && failure.Type != "" {
		return "", fmt.Errorf("JMAP method %q failed: %s", callID, failure.Type)
	}
	return "", fmt.Errorf("JMAP method %q did not create %q", callID, creationID)
}

func (r jmapResponse) requireDestroyedEmail(callID, emailID string) error {
	response, err := r.find(callID, "Email/set")
	if err != nil {
		return err
	}
	var payload struct {
		Destroyed    []string `json:"destroyed"`
		NotDestroyed map[string]struct {
			Type string `json:"type"`
		} `json:"notDestroyed"`
	}
	if err := json.Unmarshal(response[1], &payload); err != nil {
		return fmt.Errorf("decode JMAP cleanup response: %w", err)
	}
	for _, destroyedID := range payload.Destroyed {
		if destroyedID == emailID {
			return nil
		}
	}
	if failure, ok := payload.NotDestroyed[emailID]; ok && failure.Type != "" {
		return fmt.Errorf("JMAP draft cleanup failed: %s", failure.Type)
	}
	return fmt.Errorf("JMAP draft cleanup did not destroy the submitted email")
}
