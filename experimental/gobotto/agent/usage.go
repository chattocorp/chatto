package agent

// wireUsage preserves absent and null cache counts instead of converting them
// to cache misses. Both completion endpoints use this OpenRouter response shape.
type wireUsage struct {
	Input   int64    `json:"prompt_tokens"`
	Output  int64    `json:"completion_tokens"`
	Cost    *float64 `json:"cost"`
	Details struct {
		Cached *int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u wireUsage) report(provider, requestID string) Usage {
	r := Usage{Input: u.Input, Output: u.Output, Cost: u.Cost, Provider: provider, RequestID: requestID}
	if u.Details.Cached != nil {
		r.CachedInput = *u.Details.Cached
		r.CacheKnown = true
	}
	return r
}
