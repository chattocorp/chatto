package connectapi

import (
	"fmt"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func TestDisplayNamesAcrossAccountAPIs(t *testing.T) {
	env := newConnectAPITestEnv(t)
	operator := &operatorUserService{api: env.api}
	bots := &botService{api: env.api}
	ctx := withCaller(env.ctx, env.viewer)
	for i, name := range []string{"[DEV] ChattoBot", "🕹️", "👩‍💻", "「王小明」", "!!!", "Alice  Smith", "می\u200Cروم", "#️⃣"} {
		t.Run(name, func(t *testing.T) {
			human, err := operator.CreateUser(env.ctx, connect.NewRequest(&operatorv1.CreateUserRequest{
				Login: fmt.Sprintf("display-human-%d", i), DisplayName: "  " + name + "  ",
			}))
			require.NoError(t, err)
			require.Equal(t, name, human.Msg.GetMember().GetUser().GetDisplayName())
			bot, err := bots.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{
				Login: fmt.Sprintf("display-bot-%d", i), DisplayName: "  " + name + "  ",
			}))
			require.NoError(t, err)
			require.Equal(t, name, bot.Msg.GetBot().GetUser().GetDisplayName())
			for _, id := range []string{human.Msg.GetMember().GetUser().GetId(), bot.Msg.GetBot().GetUser().GetId()} {
				account, err := env.core.GetUser(env.ctx, id)
				require.NoError(t, err)
				updated, err := env.users.UpdateUserProfile(withCaller(env.ctx, account), connect.NewRequest(&apiv1.UpdateUserProfileRequest{
					UserId: id, DisplayName: stringPtr(name + "!"),
				}))
				require.NoError(t, err)
				require.Equal(t, name+"!", updated.Msg.GetUser().GetDisplayName())
			}
		})
	}
	for _, name := range []string{"Bad\nName", "Bad\u2028Name", "Bad\u202EName", "Bad\u200BName", "\u0301", "\u200D", "\u3164", "\u2800", strings.Repeat("田", 33)} {
		t.Run("reject "+name, func(t *testing.T) {
			_, err := operator.CreateUser(env.ctx, connect.NewRequest(&operatorv1.CreateUserRequest{
				Login: "invalid-display-human", DisplayName: name,
			}))
			requireConnectCode(t, err, connect.CodeInvalidArgument)
			_, err = bots.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{
				Login: "invalid-display-bot", DisplayName: name,
			}))
			requireConnectCode(t, err, connect.CodeInvalidArgument)
			_, err = env.users.UpdateUserProfile(ctx, connect.NewRequest(&apiv1.UpdateUserProfileRequest{
				UserId: env.viewer.Id, DisplayName: stringPtr(name),
			}))
			requireConnectCode(t, err, connect.CodeInvalidArgument)
		})
	}
}
