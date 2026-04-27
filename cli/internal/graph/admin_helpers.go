package graph

// Buckets whose key listings would expose secrets directly. The keys in these
// buckets ARE the secrets (auth tokens stored as KV keys, encryption-key
// owner ids), so admins listing them would gain a one-query path to user
// impersonation — violates the privacy boundary documented in
// .claude/rules/admin.md.
var sensitiveKVBuckets = map[string]struct{}{
	"KV_AUTH_TOKENS":     {},
	"KV_ENCRYPTION_KEYS": {},
}

// Key prefixes inside otherwise-inspectable buckets where the key itself is a
// short-lived secret token (the value is just metadata about the token).
// The bucket as a whole is operational metadata; only these keys are hidden.
var sensitiveKeyPrefixes = []string{
	"password_reset.",
	"account_deletion_token.",
	"email_verification.",
}
