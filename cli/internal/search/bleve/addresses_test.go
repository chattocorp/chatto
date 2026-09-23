package bleve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractAddressTermsFromCompleteMessageSource(t *testing.T) {
	terms := extractAddressTerms("See [Preview](HTTPS://DEV.preview.chatto.run/Help-Center?q=One), " +
		"write Alice.Smith@example.com, or use `other.preview.chatto.run`.")

	require.Equal(t, []string{"https://dev.preview.chatto.run/Help-Center?q=One"}, terms.urls)
	require.Equal(t, []string{"alice.smith@example.com"}, terms.emails)
	require.Contains(t, terms.hosts, "preview.chatto.run")
	require.Contains(t, terms.hosts, "example.com")
	for _, part := range []string{"dev", "preview", "chatto", "run", "help", "center", "one", "alice", "smith", "example"} {
		require.Contains(t, terms.parts, part)
	}
}

func TestAddressRecognitionRejectsNonAddresses(t *testing.T) {
	terms := extractAddressTerms("preview chatto run, localhost, 192.0.2.1, version2.0, and not-an-email@invalid")
	require.Empty(t, terms.urls)
	require.Empty(t, terms.emails)
	require.Empty(t, terms.hosts)
	require.Empty(t, terms.parts)

	require.Empty(t, canonicalHostname("localhost"))
	require.Empty(t, canonicalHostname("192.0.2.1"))
	require.Empty(t, canonicalHostname("-example.com"))
	value, _ := canonicalHTTPURL("https://192.0.2.1/path")
	require.Empty(t, value)
}

func TestAddressPartsUseDecodedURLPath(t *testing.T) {
	terms := extractAddressTerms("https://example.com/Hello%20World")
	require.Equal(t, []string{"https://example.com/Hello%20World"}, terms.urls)
	require.Contains(t, terms.parts, "hello")
	require.Contains(t, terms.parts, "world")
	require.NotContains(t, terms.parts, "20")
}
