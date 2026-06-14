//go:build test_endpoints

package linkpreview

func init() {
	allowLocalhost = true
}

func AllowLocalhostForTesting() func() {
	orig := allowLocalhost
	allowLocalhost = true
	return func() { allowLocalhost = orig }
}
