package identitybroker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

type httpBrokerServer struct {
	server   *httptest.Server
	broker   *Broker
	account  Account
	token    string
	verifier *Verifier
}

type challengeHTTPRequest struct {
	Kind string `json:"kind"`
	Role string `json:"role"`
}

type finalizeHTTPRequest struct {
	Certificate Certificate   `json:"certificate"`
	Supporting  []Certificate `json:"supporting"`
	Now         int64         `json:"now"`
}

func newHTTPBrokerServer(t *testing.T, userID string) *httpBrokerServer {
	t.Helper()
	testServer := httptest.NewUnstartedServer(nil)
	origin := "http://" + testServer.Listener.Addr().String()
	broker, err := NewBroker(origin)
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	token, err := NewOpaqueID(24)
	if err != nil {
		t.Fatalf("NewOpaqueID: %v", err)
	}
	h := &httpBrokerServer{
		server:  testServer,
		broker:  broker,
		account: Account{Origin: broker.Origin(), UserID: userID},
		token:   token,
	}
	testServer.Config.Handler = h.handler()
	testServer.Start()
	if testServer.URL != broker.Origin() {
		t.Fatalf("test server origin = %q, broker origin = %q", testServer.URL, broker.Origin())
	}
	t.Cleanup(testServer.Close)
	return h
}

func (h *httpBrokerServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/chatto-identity-broker", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, h.broker.Discovery())
	})
	mux.HandleFunc("POST /challenge", func(w http.ResponseWriter, r *http.Request) {
		if !h.authenticate(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		var request challengeHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		challenge, err := h.broker.IssueChallenge(h.account, request.Kind, request.Role, testNow)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, challenge)
	})
	mux.HandleFunc("POST /approve", func(w http.ResponseWriter, r *http.Request) {
		if !h.authenticate(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		var request CeremonyRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		approval, err := h.broker.Approve(h.account, request, testNow.Add(time.Minute))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, approval)
	})
	mux.HandleFunc("POST /finalize", func(w http.ResponseWriter, r *http.Request) {
		if !h.authenticate(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
			return
		}
		var request finalizeHTTPRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if h.verifier == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verifier unavailable"})
			return
		}
		if err := h.broker.Finalize(request.Certificate, h.verifier, request.Supporting, time.Unix(request.Now, 0)); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"finalized": true})
	})
	mux.HandleFunc("GET /certificates", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, h.broker.Certificates())
	})
	return mux
}

func (h *httpBrokerServer) authenticate(r *http.Request) bool {
	return r.Header.Get("Authorization") == "Bearer "+h.token
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func httpJSON[T any](t *testing.T, method, rawURL, token string, requestBody any) T {
	t.Helper()
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatalf("marshal %s: %v", rawURL, err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		t.Fatalf("NewRequest(%s): %v", rawURL, err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, rawURL, err)
	}
	defer response.Body.Close()
	responseBytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", rawURL, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s status = %d: %s", method, rawURL, response.StatusCode, strings.TrimSpace(string(responseBytes)))
	}
	var result T
	if err := json.Unmarshal(responseBytes, &result); err != nil {
		t.Fatalf("decode %s: %v", rawURL, err)
	}
	return result
}

func discoverHTTPBrokers(t *testing.T, brokers []*httpBrokerServer) *TrustStore {
	t.Helper()
	trust := NewTrustStore()
	for _, broker := range brokers {
		discovery := httpJSON[DiscoveryKey](t, http.MethodGet, broker.server.URL+"/.well-known/chatto-identity-broker", "", nil)
		if err := trust.Add(discovery); err != nil {
			t.Fatalf("trust.Add(%s): %v", broker.server.URL, err)
		}
	}
	return trust
}

func httpChallenge(t *testing.T, broker *httpBrokerServer, kind, role string) Challenge {
	t.Helper()
	return httpJSON[Challenge](t, http.MethodPost, broker.server.URL+"/challenge", broker.token, challengeHTTPRequest{Kind: kind, Role: role})
}

func httpApprovals(t *testing.T, request CeremonyRequest, brokers ...*httpBrokerServer) []Approval {
	t.Helper()
	approvals := make([]Approval, 0, len(brokers))
	for _, broker := range brokers {
		approvals = append(approvals, httpJSON[Approval](t, http.MethodPost, broker.server.URL+"/approve", broker.token, request))
	}
	return approvals
}

func httpFinalize(t *testing.T, broker *httpBrokerServer, certificate Certificate, supporting []Certificate, now time.Time) {
	t.Helper()
	httpJSON[map[string]bool](t, http.MethodPost, broker.server.URL+"/finalize", broker.token, finalizeHTTPRequest{
		Certificate: certificate,
		Supporting:  supporting,
		Now:         now.Unix(),
	})
}

func httpGenesis(t *testing.T, first, second *httpBrokerServer, now time.Time) (Certificate, string) {
	t.Helper()
	groupID, err := NewOpaqueID(32)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := NewGenesisStatement(groupID, []Challenge{
		httpChallenge(t, first, KindGenesis, RoleFounder),
		httpChallenge(t, second, KindGenesis, RoleFounder),
	}, publicKey, now, 72*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request, err := SignCeremony(statement, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return Certificate{Request: request, Approvals: httpApprovals(t, request, first, second)}, groupID
}

func httpMembership(t *testing.T, groupID string, target, sponsorA, sponsorB *httpBrokerServer, refs []SponsorRef, now time.Time) Certificate {
	t.Helper()
	publicKey, privateKey, err := NewCeremonyKey()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := NewMembershipStatement(
		groupID,
		httpChallenge(t, target, KindMembership, RoleTarget),
		[]Challenge{
			httpChallenge(t, sponsorA, KindMembership, RoleSponsor),
			httpChallenge(t, sponsorB, KindMembership, RoleSponsor),
		},
		refs,
		publicKey,
		now,
		72*time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := SignCeremony(statement, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return Certificate{Request: request, Approvals: httpApprovals(t, request, target, sponsorA, sponsorB)}
}

func TestHTTPBrokerScalesToTwentyServersAndCleanClient(t *testing.T) {
	const serverCount = 20
	brokers := make([]*httpBrokerServer, 0, serverCount)
	for i := range serverCount {
		brokers = append(brokers, newHTTPBrokerServer(t, fmt.Sprintf("user-%02d", i)))
	}

	serverTrust := discoverHTTPBrokers(t, brokers)
	serverVerifier := NewVerifier(serverTrust)
	for _, broker := range brokers {
		broker.verifier = serverVerifier
	}

	genesis, groupID := httpGenesis(t, brokers[0], brokers[1], testNow)
	httpFinalize(t, brokers[0], genesis, nil, testNow)
	httpFinalize(t, brokers[1], genesis, nil, testNow)
	genesisID, err := StatementID(genesis.Request.Statement)
	if err != nil {
		t.Fatal(err)
	}
	sponsorRefs := []SponsorRef{
		{Account: brokers[0].account, CredentialID: CredentialID(genesisID, brokers[0].account)},
		{Account: brokers[1].account, CredentialID: CredentialID(genesisID, brokers[1].account)},
	}

	for i := 2; i < serverCount; i++ {
		issuedAt := testNow.Add(time.Duration(i) * time.Second)
		membership := httpMembership(t, groupID, brokers[i], brokers[0], brokers[1], sponsorRefs, issuedAt)
		for _, participant := range []*httpBrokerServer{brokers[i], brokers[0], brokers[1]} {
			httpFinalize(t, participant, membership, []Certificate{genesis}, issuedAt)
		}
	}

	// A new device builds a new trust store and fetches only public artifacts;
	// it receives no ceremony private key or prior client state.
	cleanTrust := discoverHTTPBrokers(t, brokers)
	cleanVerifier := NewVerifier(cleanTrust)
	var bundle []Certificate
	for _, broker := range brokers {
		certificates := httpJSON[[]Certificate](t, http.MethodGet, broker.server.URL+"/certificates", "", nil)
		bundle = append(bundle, certificates...)
	}
	group, err := cleanVerifier.Reconstruct(bundle, testNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("clean Reconstruct: %v", err)
	}
	if got := len(group.MemberAccounts()); got != serverCount {
		t.Fatalf("member count = %d, want %d", got, serverCount)
	}

	uniqueCertificates := map[string]struct{}{}
	for _, certificate := range bundle {
		id, err := StatementID(certificate.Request.Statement)
		if err != nil {
			t.Fatal(err)
		}
		uniqueCertificates[id] = struct{}{}
	}
	if got, want := len(uniqueCertificates), serverCount-1; got != want {
		t.Fatalf("unique certificate count = %d, want linear %d", got, want)
	}

	wantAccounts := make([]Account, 0, serverCount)
	for _, broker := range brokers {
		wantAccounts = append(wantAccounts, broker.account)
	}
	// Both lists use the protocol's stable account ordering.
	slices.SortFunc(wantAccounts, func(a, b Account) int { return strings.Compare(a.key(), b.key()) })
	if got := group.MemberAccounts(); !reflect.DeepEqual(got, wantAccounts) {
		t.Fatalf("clean members = %#v, want %#v", got, wantAccounts)
	}
}
