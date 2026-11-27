package connectapi

import (
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
)

// The fixture contains the stored NotificationSignal descriptor from v0.6.0.
// Generate it from proto/ with:
//
//	buf build '../.git#tag=v0.6.0,subdir=proto' --exclude-source-info --as-file-descriptor-set \
//	  --type chatto.core.notification.v1.NotificationSignal \
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
