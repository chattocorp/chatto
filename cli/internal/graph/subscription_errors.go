package graph

import "github.com/vektah/gqlparser/v2/gqlerror"

const myEventsFullRefreshRequiredCode = "MY_EVENTS_FULL_REFRESH_REQUIRED"

func myEventsFullRefreshRequiredError(err error) error {
	return &gqlerror.Error{
		Message: "myEvents cursor requires a full refresh",
		Err:     err,
		Extensions: map[string]any{
			"code": myEventsFullRefreshRequiredCode,
		},
	}
}
