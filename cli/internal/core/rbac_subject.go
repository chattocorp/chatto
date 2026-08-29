package core

// ObjectIdAny identifies a server-scope decision in permission explanations.
const ObjectIdAny = "any"

// IsUserSubject reports whether an RBAC subject is a user ID. User IDs start
// with the U prefix. Other subject values identify roles.
func IsUserSubject(subject string) bool {
	return len(subject) > 0 && subject[0] == 'U'
}
