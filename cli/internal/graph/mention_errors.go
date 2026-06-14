package graph

import (
	"fmt"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

func largeMentionConfirmationError(recipientCount int, token string) error {
	return &gqlerror.Error{
		Message: fmt.Sprintf("This message would notify %d people. Please confirm before sending.", recipientCount),
		Extensions: map[string]any{
			"code":                     "MENTION_CONFIRMATION_REQUIRED",
			"recipientCount":           recipientCount,
			"mentionConfirmationToken": token,
		},
	}
}
