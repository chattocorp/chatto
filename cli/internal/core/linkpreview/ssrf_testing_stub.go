//go:build !test_endpoints

package linkpreview

func AllowLocalhostForTesting() func() {
	orig := allowLocalhost
	allowLocalhost = true
	return func() { allowLocalhost = orig }
}
