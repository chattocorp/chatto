package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

// searchFlags translates structured tool fields to means flags in stable order.
// Values retain their JSON types (including large integers and JSON-like strings).
// The caller places the free-text query after --; no argument is shell-expanded.
func (a arguments) searchFlags() ([]string, error) {
	flags := []string{}
	for _, key := range sortedKeys(a.Filters) {
		if !validFilterKey(key) {
			return nil, errors.New("Filter keys must be nonempty data field names without '=' or NUL")
		}
		var value bytes.Buffer
		if err := json.Compact(&value, a.Filters[key]); err != nil {
			return nil, errors.New("Filter values must be valid JSON")
		}
		flags = append(flags, "--filter", key+"="+value.String())
	}
	for _, group := range []struct {
		flag   string
		fields map[string]string
	}{
		{"--after", a.After}, {"--before", a.Before},
		{"--on-or-after", a.OnOrAfter}, {"--on-or-before", a.OnOrBefore},
	} {
		for _, key := range sortedKeys(group.fields) {
			if !validFilterKey(key) {
				return nil, errors.New("Filter keys must be nonempty data field names without '=' or NUL")
			}
			value := group.fields[key]
			if _, err := time.Parse(time.RFC3339, value); err != nil {
				return nil, errors.New("Datetime filters require RFC 3339 timestamps with a timezone, not date-only values")
			}
			flags = append(flags, group.flag, key+"="+value)
		}
	}
	return flags, nil
}

func validFilterKey(key string) bool {
	return strings.TrimSpace(key) != "" && !strings.ContainsAny(key, "=\x00")
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
