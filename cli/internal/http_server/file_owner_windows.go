package http_server

import "os"

// Windows file metadata does not expose Unix user and group IDs. Returning
// false makes the Unix-socket operator API fail closed if it is enabled.
func fileOwnerIDs(os.FileInfo) (uint32, uint32, bool) {
	return 0, 0, false
}
