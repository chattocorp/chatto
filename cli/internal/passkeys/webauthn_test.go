// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later

package passkeys

import (
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestNewDerivesExactRelyingPartyFromPublicURL(t *testing.T) {
	enabled := true
	cfg := config.ChattoConfig{
		Webserver: config.WebserverConfig{URL: "https://chat.example.test:8443"},
		Auth:      config.AuthConfig{Passkeys: config.PasskeysConfig{Enabled: &enabled}},
	}
	wa, err := New(cfg, "Example Chat")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if wa.Config.RPID != "chat.example.test" {
		t.Fatalf("RPID = %q, want chat.example.test", wa.Config.RPID)
	}
	if got := wa.Config.RPOrigins; len(got) != 1 || got[0] != "https://chat.example.test:8443" {
		t.Fatalf("RPOrigins = %#v", got)
	}
}
