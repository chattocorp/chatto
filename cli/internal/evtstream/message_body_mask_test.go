package evtstream

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestMessageBodyMaskWirePresence(t *testing.T) {
	for _, tc := range []struct {
		name string
		mask *fieldmaskpb.FieldMask
		want MessageBodyFields
	}{
		{"historical replacement", nil, MessageBodyFields{true, true, true, true}},
		{"no changed fields", &fieldmaskpb.FieldMask{}, MessageBodyFields{}},
		{"clear description list", &fieldmaskpb.FieldMask{Paths: []string{"attachment_descriptions"}}, MessageBodyFields{Descriptions: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := proto.Marshal(&evtv1.MessageBodyEvent{Body: &evtv1.MessageBody{}, UpdateMask: tc.mask})
			require.NoError(t, err)
			var decoded evtv1.MessageBodyEvent
			require.NoError(t, proto.Unmarshal(wire, &decoded))
			fields, err := ParseMessageBodyMask(decoded.GetUpdateMask())
			require.NoError(t, err)
			require.Equal(t, tc.want, fields)
		})
	}
}

func TestMessageBodyMaskRejectsUnsupportedAndIncompleteUpdates(t *testing.T) {
	for _, paths := range [][]string{
		{"future_field"}, {"*"}, {"attachment_descriptions.asset_id"},
		{"encrypted_body"}, {"encryption_nonce"}, {"asset_ids"}, {"attachments"},
	} {
		_, err := ParseMessageBodyMask(&fieldmaskpb.FieldMask{Paths: paths})
		require.Error(t, err, "mask %v", paths)
	}
}
