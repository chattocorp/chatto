package connectapi

import (
	"os"
	"testing"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/chattocorp/chatto/cli/internal/config"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
)

// The fixture contains the released v0.6.0 descriptors for the three voice
// request messages and the stored NotificationSignal, including field rules.
// Generate it from proto/ with:
//
//	buf build '../.git#tag=v0.6.0,subdir=proto' --exclude-source-info --as-file-descriptor-set \
//	  --type chatto.api.v1.JoinCallRequest,chatto.api.v1.CreateCallTokenRequest,chatto.api.v1.LeaveCallRequest,chatto.core.notification.v1.NotificationSignal \
//	  -o ../cli/internal/connectapi/testdata/v0.6.0-call-notifications.binpb
func oldCallMessage(t *testing.T, current proto.Message) *dynamicpb.Message {
	t.Helper()
	data, err := os.ReadFile("testdata/v0.6.0-call-notifications.binpb")
	require.NoError(t, err)
	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(data, &set))
	files, err := protodesc.NewFiles(&set)
	require.NoError(t, err)
	desc, err := files.FindDescriptorByName(current.ProtoReflect().Descriptor().FullName())
	require.NoError(t, err)
	return dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
}

func TestCallRequestsDuringRollingUpgrade(t *testing.T) {
	call := &apiv1.CallReference{RoomId: "room", CallId: "call"}
	for _, request := range []struct{ guarded, ordinary proto.Message }{
		{&apiv1.JoinCallRequest{Call: call}, &apiv1.JoinCallRequest{RoomId: "room"}},
		{&apiv1.CreateCallTokenRequest{Call: call}, &apiv1.CreateCallTokenRequest{RoomId: "room"}},
		{&apiv1.LeaveCallRequest{Call: call}, &apiv1.LeaveCallRequest{RoomId: "room"}},
	} {
		t.Run(string(request.guarded.ProtoReflect().Descriptor().Name()), func(t *testing.T) {
			old := oldCallMessage(t, request.guarded)
			require.NoError(t, protovalidate.Validate(request.guarded))
			guarded, err := proto.Marshal(request.guarded)
			require.NoError(t, err)
			require.NoError(t, proto.Unmarshal(guarded, old))
			require.Error(t, protovalidate.Validate(old), "old replica must reject the guarded action")
			jsonData, err := protojson.Marshal(request.guarded)
			require.NoError(t, err)
			require.NoError(t, (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(jsonData, old))
			require.Error(t, protovalidate.Validate(old))

			ordinary, err := proto.Marshal(request.ordinary)
			require.NoError(t, err)
			require.NoError(t, proto.Unmarshal(ordinary, old))
			require.NoError(t, protovalidate.Validate(old))
			oldBytes, err := proto.Marshal(old)
			require.NoError(t, err)
			decoded := request.ordinary.ProtoReflect().New().Interface()
			require.NoError(t, proto.Unmarshal(oldBytes, decoded))
			require.NoError(t, protovalidate.Validate(decoded))
			require.True(t, proto.Equal(request.ordinary, decoded))

			both := proto.Clone(request.guarded)
			proto.Merge(both, request.ordinary)
			require.Error(t, protovalidate.Validate(both))
			require.Error(t, protovalidate.Validate(both.ProtoReflect().New().Interface()))
		})
	}
}

func TestIncomingCallSignalDuringRollingUpgrade(t *testing.T) {
	signal := &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_IncomingCall{IncomingCall: &notificationv1.IncomingCall{RoomId: "room", CallId: "call"}}}
	data, err := proto.Marshal(signal)
	require.NoError(t, err)
	old := oldCallMessage(t, signal)
	require.NoError(t, proto.Unmarshal(data, old))
	require.Nil(t, old.WhichOneof(old.Descriptor().Oneofs().ByName("kind")))
	// This is how the old generated reader exposes the new oneof member to
	// the list handler. The old reader preserves its bytes on a round trip.
	oldView := &notificationv1.NotificationSignal{}
	oldView.ProtoReflect().SetUnknown(old.GetUnknown())
	require.Equal(t, connect.CodeUnimplemented, errorCode(requireSupportedNotificationSignals(&notificationv1.NotificationOccurrence{Signal: oldView})))
	encoded, err := proto.Marshal(old)
	require.NoError(t, err)
	upgraded := &notificationv1.NotificationSignal{}
	require.NoError(t, proto.Unmarshal(encoded, upgraded))
	require.True(t, proto.Equal(signal, upgraded))
	require.NoError(t, requireSupportedNotificationSignals(&notificationv1.NotificationOccurrence{Signal: upgraded}))
}

func TestGuardedVoiceActionsAfterCallEnds(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("guarded-voice")
	env.api.config.LiveKit = config.LiveKitConfig{Enabled: true, URL: "ws://livekit.test", APIKey: "test-key", APISecret: "test-secret", ServerID: "server"}
	ctx := withCaller(env.ctx, env.viewer)
	join := func() *apiv1.CallReference {
		_, err := env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id}))
		require.NoError(t, err)
		call, active, err := env.core.GetActiveCall(room.Id)
		require.NoError(t, err)
		require.True(t, active)
		return &apiv1.CallReference{RoomId: room.Id, CallId: call.CallID}
	}
	first := join()
	_, err := env.voice.LeaveCall(ctx, connect.NewRequest(&apiv1.LeaveCallRequest{Call: first}))
	require.NoError(t, err)
	checkStale := func() {
		_, err := env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{Call: first}))
		require.Equal(t, connect.CodeNotFound, errorCode(err))
		_, err = env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{Call: first}))
		require.Equal(t, connect.CodeNotFound, errorCode(err))
		_, err = env.voice.LeaveCall(ctx, connect.NewRequest(&apiv1.LeaveCallRequest{Call: first}))
		require.NoError(t, err)
	}
	checkStale()
	_, active, err := env.core.GetActiveCall(room.Id)
	require.NoError(t, err)
	require.False(t, active)
	_, err = env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{RoomId: room.Id}))
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
	replacement := join()
	require.NotEqual(t, first.CallId, replacement.CallId)
	checkStale()
	snapshot, err := env.core.GetCallSnapshot(room.Id)
	require.NoError(t, err)
	require.Equal(t, replacement.CallId, snapshot.Call.CallID)
	require.Len(t, snapshot.Participants, 1)
	require.Equal(t, env.viewer.Id, snapshot.Participants[0].UserID)
	token, err := env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{Call: replacement}))
	require.NoError(t, err)
	require.Equal(t, replacement.CallId, token.Msg.GetCallId())
}
