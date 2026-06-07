package model

// Reaction represents an emoji aggregate for a message. UserIDs is internal:
// GraphQL exposes a bounded user preview through the field resolver while
// Count remains the total reaction count.
type Reaction struct {
	Emoji      string   `json:"emoji"`
	Count      int32    `json:"count"`
	HasReacted bool     `json:"hasReacted"`
	UserIDs    []string `json:"-"`
}
