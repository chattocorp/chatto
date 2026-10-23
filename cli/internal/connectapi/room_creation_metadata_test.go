package connectapi

import (
	"testing"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAPIRoomCreationMetadata(t *testing.T) {
	t.Parallel()
	for _, actor := range []string{"creator", "system", ""} {
		t.Run(actor, func(t *testing.T) {
			t.Parallel()
			room := apiRoom(&evtv1.Room{CreatedAt: &timestamppb.Timestamp{Seconds: 123}, CreatedBy: actor})
			require.Equal(t, int64(123), room.GetCreatedAt().GetSeconds())
			require.Equal(t, actor, room.GetCreatedBy())
			require.Equal(t, actor == "", room.CreatedBy == nil)
		})
	}
	require.Nil(t, apiRoom(&evtv1.Room{}).CreatedAt)
}
