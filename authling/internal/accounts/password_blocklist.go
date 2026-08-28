package accounts

import "strings"

// commonPasswords is a deliberately small built-in baseline. Entries are
// lowercase because password creation checks them case-insensitively; password
// verification remains case-sensitive.
var commonPasswords = map[string]struct{}{
	"00000000":       {},
	"11111111":       {},
	"12345678":       {},
	"123456789":      {},
	"1234567890":     {},
	"12345678910":    {},
	"123456789abc":   {},
	"abc12345":       {},
	"admin123":       {},
	"baseball":       {},
	"football":       {},
	"iloveyou":       {},
	"letmein123":     {},
	"monkey123":      {},
	"password":       {},
	"password1":      {},
	"password12":     {},
	"password123":    {},
	"password1234":   {},
	"password12345":  {},
	"password123456": {},
	"passw0rd":       {},
	"princess":       {},
	"qwerty123":      {},
	"qwerty12345":    {},
	"qwertyuiop":     {},
	"sunshine":       {},
	"trustno1":       {},
	"welcome1":       {},
}

func isCommonPassword(password string) bool {
	_, blocked := commonPasswords[strings.ToLower(password)]
	return blocked
}
