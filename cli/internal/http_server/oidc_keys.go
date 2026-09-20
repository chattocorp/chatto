package http_server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// oidcKeyTransport is used only by the verifier's remote key fetcher. RFC 7517
// section 5 permits mixed key sets: unsupported key types and curves must not
// prevent use of a supported signing key. go-oidc filters unsupported algorithms
// but its current go-jose parser fails the whole set on unknown EC curves.
// Recognized keys remain unchanged and receive normal library validation.
type oidcKeyTransport struct{ base http.RoundTripper }

func (t oidcKeyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil || response.StatusCode != http.StatusOK {
		return response, err
	}
	defer response.Body.Close()
	const maxKeySetBytes = 1 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxKeySetBytes+1))
	if err != nil || len(body) > maxKeySetBytes {
		return nil, errors.New("OIDC key set exceeds limit or cannot be read")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, errors.New("invalid OIDC key set")
	}
	var keys []json.RawMessage
	if err := json.Unmarshal(document["keys"], &keys); err != nil {
		return nil, errors.New("invalid OIDC key list")
	}
	kept := make([]json.RawMessage, 0, len(keys))
	for _, key := range keys {
		var metadata struct {
			Type  string `json:"kty"`
			Curve string `json:"crv"`
		}
		if err := json.Unmarshal(key, &metadata); err != nil {
			return nil, errors.New("invalid OIDC key metadata")
		}
		switch metadata.Type {
		case "RSA":
		case "EC":
			if metadata.Curve != "P-256" && metadata.Curve != "P-384" && metadata.Curve != "P-521" {
				continue
			}
		case "OKP":
			if metadata.Curve != "Ed25519" {
				continue
			}
		default:
			continue
		}
		kept = append(kept, key)
	}
	document["keys"], err = json.Marshal(kept)
	if err != nil {
		return nil, errors.New("cannot encode OIDC keys")
	}
	body, err = json.Marshal(document)
	if err != nil {
		return nil, errors.New("cannot encode OIDC key set")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	response.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return response, nil
}
