package interchange

import "os"

// The operator socket is Unix-only. Do not accept files where link counts cannot
// be checked by this directory reader; remote archive validation is independent.
func singleLink(os.FileInfo) bool { return false }
