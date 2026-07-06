package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func (c *ChattoCore) runtimeTokenKey(prefix, token string) string {
	scope := strings.TrimSuffix(prefix, ".")
	return prefix + c.runtimeTokenHash(scope, token)
}

func (c *ChattoCore) runtimeTokenHash(scope, token string) string {
	mac := hmac.New(sha256.New, []byte(c.config.SecretKey))
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
