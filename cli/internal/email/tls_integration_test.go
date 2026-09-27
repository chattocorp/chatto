package email_test

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	smtpmock "github.com/mocktools/go-smtp-mock/v2"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/email"
)

// selfSignedCertificate returns a certificate for 127.0.0.1 that no system
// trust store accepts.
func selfSignedCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "chatto-test-smtp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// startTLSProxy starts a TLS-terminating proxy in front of a plaintext mock
// SMTP server and returns the proxy port. With implicit set, the proxy
// requires TLS from the first byte (SMTPS). Otherwise it offers STARTTLS and
// forwards the session to the backend after the TLS handshake.
func startTLSProxy(t *testing.T, backend *smtpmock.Server, implicit bool) int {
	t.Helper()
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{selfSignedCertificate(t)}, MinVersion: tls.VersionTLS12}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = listener.Close()
		wg.Wait()
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				if implicit {
					tlsConn := tls.Server(conn, tlsConfig)
					if tlsConn.Handshake() != nil {
						return
					}
					pipeToBackend(tlsConn, backend, false)
					return
				}
				serveSTARTTLS(conn, tlsConfig, backend)
			}()
		}
	}()

	return listener.Addr().(*net.TCPAddr).Port
}

// serveSTARTTLS answers the plaintext part of a STARTTLS session, upgrades the
// connection, and forwards the encrypted session to the backend.
func serveSTARTTLS(conn net.Conn, tlsConfig *tls.Config, backend *smtpmock.Server) {
	reader := bufio.NewReader(conn)
	if _, err := io.WriteString(conn, "220 proxy ESMTP\r\n"); err != nil {
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
			_, err = io.WriteString(conn, "250-proxy\r\n250 STARTTLS\r\n")
		case command == "STARTTLS":
			if _, err := io.WriteString(conn, "220 Ready to start TLS\r\n"); err != nil {
				return
			}
			tlsConn := tls.Server(conn, tlsConfig)
			if tlsConn.Handshake() != nil {
				return
			}
			pipeToBackend(tlsConn, backend, true)
			return
		case command == "QUIT":
			_, _ = io.WriteString(conn, "221 Bye\r\n")
			return
		default:
			_, err = io.WriteString(conn, "502 Command not implemented\r\n")
		}
		if err != nil {
			return
		}
	}
}

// pipeToBackend copies the client session to the mock SMTP server. After
// STARTTLS the client does not expect a second greeting, so skipGreeting
// consumes the backend greeting first.
func pipeToBackend(client net.Conn, backend *smtpmock.Server, skipGreeting bool) {
	upstream, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(backend.PortNumber())))
	if err != nil {
		return
	}
	defer upstream.Close()
	var upstreamReader io.Reader = upstream
	if skipGreeting {
		buffered := bufio.NewReader(upstream)
		if _, err := buffered.ReadString('\n'); err != nil {
			return
		}
		upstreamReader = buffered
	}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstreamReader); done <- struct{}{} }()
	<-done
}

func TestMailer_Integration_TLSCertificateVerification(t *testing.T) {
	tests := []struct {
		name       string
		implicit   bool
		policy     config.SMTPTLSPolicy
		skipVerify bool
		wantSent   bool
	}{
		{name: "mandatory STARTTLS rejects untrusted certificate", policy: config.SMTPTLSMandatory},
		{name: "opportunistic STARTTLS rejects untrusted certificate", policy: config.SMTPTLSOpportunistic},
		{name: "mandatory STARTTLS with skip verify delivers", policy: config.SMTPTLSMandatory, skipVerify: true, wantSent: true},
		{name: "implicit TLS rejects untrusted certificate", implicit: true, policy: config.SMTPTLSImplicit},
		{name: "implicit TLS with skip verify delivers", implicit: true, policy: config.SMTPTLSImplicit, skipVerify: true, wantSent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := startMockServer(t, smtpmock.ConfigurationAttr{})
			port := startTLSProxy(t, backend, tt.implicit)

			mailer := email.NewMailer(config.SMTPConfig{
				Enabled:       true,
				Host:          "127.0.0.1",
				Port:          port,
				TLS:           tt.policy,
				TLSSkipVerify: tt.skipVerify,
				From:          "sender@example.com",
			})
			err := mailer.Send(email.Message{
				To:      "recipient@example.com",
				Subject: "TLS test",
				Body:    "Testing TLS certificate verification",
			})

			if !tt.wantSent {
				if err == nil {
					t.Fatal("expected certificate verification error, got nil")
				}
				if !strings.Contains(err.Error(), "certificate") {
					t.Errorf("expected certificate-related error, got %q", err.Error())
				}
				if strings.Contains(err.Error(), "@example.com") {
					t.Errorf("error contains an email address: %q", err.Error())
				}
				if messages := backend.Messages(); len(messages) != 0 {
					t.Fatalf("expected no delivered messages, got %d", len(messages))
				}
				return
			}

			if err != nil {
				t.Fatalf("Send() failed: %v", err)
			}
			messages, err := backend.WaitForMessages(1, time.Second)
			if err != nil {
				t.Fatalf("WaitForMessages failed: %v", err)
			}
			if len(messages) != 1 || !strings.Contains(messages[0].MsgRequest(), "TLS test") {
				t.Fatalf("expected the message to reach the server, got %d messages", len(messages))
			}
		})
	}
}
