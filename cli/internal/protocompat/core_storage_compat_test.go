// SPDX-FileCopyrightText: 2024-present Chatto contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package protocompat_test

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	_ "hmans.de/chatto/internal/pb/chatto/core/cache_state/v1"
	_ "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	_ "hmans.de/chatto/internal/pb/chatto/core/key_material/v1"
	_ "hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	_ "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
	_ "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
)

//go:embed testdata/*.binpb
var compatibilityFixtures embed.FS

var storedMessageRoots = []protoreflect.FullName{
	"chatto.core.v1.Event",
	"chatto.core.v1.NotificationEvent",
	"chatto.core.v1.CredentialUsageState",
	"chatto.core.v1.PushSubscription",
	"chatto.core.v1.UserDataEncryptionKey",
	"chatto.core.v1.CachedLinkPreview",
	"chatto.core.v1.NotificationUnreadMarker",
	"chatto.core.v1.CookieSession",
	"chatto.core.v1.UserKeyEncryptionKey",
	"chatto.core.v1.UserPresence",
	"chatto.core.v1.RoomLayout",
	"chatto.core.v1.ThreadMetadata",
	"chatto.core.v1.VerifiedEmail",
	"chatto.core.v1.VideoProcessingState",
	"chatto.core.v1.ProjectionSnapshotGeneration",
	"chatto.core.v1.ProjectionSnapshotPointer",
}

var packageExceptions = map[string]string{
	"AllMentionReceived":           "chatto.core.notification.v1",
	"DirectMentionReceived":        "chatto.core.notification.v1",
	"DirectMessageReceived":        "chatto.core.notification.v1",
	"DMMessageNotification":        "chatto.core.notification.v1",
	"FollowedRoomActivity":         "chatto.core.notification.v1",
	"FollowedThreadActivity":       "chatto.core.notification.v1",
	"HereMentionReceived":          "chatto.core.notification.v1",
	"MentionNotification":          "chatto.core.notification.v1",
	"Notification":                 "chatto.core.notification.v1",
	"NotificationAlertResolved":    "chatto.core.notification.v1",
	"NotificationAttentionLevel":   "chatto.core.notification.v1",
	"NotificationEvent":            "chatto.core.notification.v1",
	"NotificationMessageReference": "chatto.core.notification.v1",
	"NotificationOccurrence":       "chatto.core.notification.v1",
	"NotificationRead":             "chatto.core.notification.v1",
	"NotificationRemoved":          "chatto.core.notification.v1",
	"NotificationSignal":           "chatto.core.notification.v1",
	"NotificationSignalled":        "chatto.core.notification.v1",
	"ReactionReceived":             "chatto.core.notification.v1",
	"ReplyNotification":            "chatto.core.notification.v1",
	"ReplyReceived":                "chatto.core.notification.v1",
	"RoleMentionReceived":          "chatto.core.notification.v1",
	"RoomMessageNotification":      "chatto.core.notification.v1",
	"RoomMessageReceived":          "chatto.core.notification.v1",

	"CachedLinkPreview":        "chatto.core.runtime_state.v1",
	"CookieSession":            "chatto.core.runtime_state.v1",
	"CredentialUsageState":     "chatto.core.runtime_state.v1",
	"NotificationUnreadMarker": "chatto.core.runtime_state.v1",
	"PushSubscription":         "chatto.core.runtime_state.v1",
	"RoomLayout":               "chatto.core.runtime_state.v1",
	"ThreadMetadata":           "chatto.core.runtime_state.v1",
	"UserDataEncryptionKey":    "chatto.core.runtime_state.v1",
	"VerifiedEmail":            "chatto.core.runtime_state.v1",
	"VideoProcessingState":     "chatto.core.runtime_state.v1",
	"VideoStatus":              "chatto.core.runtime_state.v1",
	"VideoVariant":             "chatto.core.runtime_state.v1",

	"UserPresence":       "chatto.core.cache_state.v1",
	"UserPresenceStatus": "chatto.core.cache_state.v1",

	"UserKeyEncryptionKey": "chatto.core.key_material.v1",

	"ProjectionSnapshotGeneration": "chatto.core.projection.v1",
	"ProjectionSnapshotPointer":    "chatto.core.projection.v1",
}

func TestCoreStorageWireCompatibility(t *testing.T) {
	fixtureNames := []string{
		"testdata/v0.4.20.binpb",
		"testdata/pre-refactor-80a112609.binpb",
	}

	for _, fixtureName := range fixtureNames {
		t.Run(fixtureName, func(t *testing.T) {
			files := loadDescriptorSet(t, fixtureName)
			compared := 0
			for _, oldName := range storedMessageRoots {
				oldDescriptor, err := files.FindDescriptorByName(oldName)
				if err == protoregistry.NotFound {
					continue
				}
				if err != nil {
					t.Fatalf("find %s: %v", oldName, err)
				}
				oldMessage, ok := oldDescriptor.(protoreflect.MessageDescriptor)
				if !ok {
					t.Fatalf("%s is not a message", oldName)
				}

				newMessage := findCurrentMessage(t, relocatedName(oldName))
				compareMessageDescriptors(t, oldMessage, newMessage, map[string]bool{})
				verifyRepresentativePayloads(t, oldMessage, newMessage)
				compared++
			}

			if compared == 0 {
				t.Fatal("fixture contains no known stored core messages")
			}
		})
	}
}

func loadDescriptorSet(t *testing.T, name string) *protoregistry.Files {
	t.Helper()
	data, err := compatibilityFixtures.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, set); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		t.Fatalf("build %s descriptor registry: %v", name, err)
	}
	return files
}

func findCurrentMessage(t *testing.T, name protoreflect.FullName) protoreflect.MessageDescriptor {
	t.Helper()
	descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
	if err != nil {
		t.Fatalf("find current %s: %v", name, err)
	}
	message, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("current %s is not a message", name)
	}
	return message
}

func relocatedName(oldName protoreflect.FullName) protoreflect.FullName {
	const oldPrefix = "chatto.core.v1."
	suffix := strings.TrimPrefix(string(oldName), oldPrefix)
	topLevelName := strings.SplitN(suffix, ".", 2)[0]
	newPackage := packageExceptions[topLevelName]
	if newPackage == "" {
		newPackage = "chatto.core.evt.v1"
	}
	return protoreflect.FullName(newPackage + "." + suffix)
}

func compareMessageDescriptors(t *testing.T, oldMessage, newMessage protoreflect.MessageDescriptor, seen map[string]bool) {
	t.Helper()
	pair := string(oldMessage.FullName()) + " -> " + string(newMessage.FullName())
	if seen[pair] {
		return
	}
	seen[pair] = true

	oldFields := oldMessage.Fields()
	for i := 0; i < oldFields.Len(); i++ {
		oldField := oldFields.Get(i)
		newField := newMessage.Fields().ByNumber(oldField.Number())
		if newField == nil {
			t.Errorf("%s: stored field %s (%d) was removed", pair, oldField.Name(), oldField.Number())
			continue
		}

		fieldPath := fmt.Sprintf("%s field %s (%d)", pair, oldField.Name(), oldField.Number())
		if oldField.Kind() != newField.Kind() {
			t.Errorf("%s: kind changed from %s to %s", fieldPath, oldField.Kind(), newField.Kind())
		}
		if oldField.Cardinality() != newField.Cardinality() {
			t.Errorf("%s: cardinality changed from %s to %s", fieldPath, oldField.Cardinality(), newField.Cardinality())
		}
		if oldField.IsMap() != newField.IsMap() {
			t.Errorf("%s: map shape changed", fieldPath)
		}
		if oldField.IsPacked() != newField.IsPacked() {
			t.Errorf("%s: packed encoding changed", fieldPath)
		}

		oldOneof := oldField.ContainingOneof()
		newOneof := newField.ContainingOneof()
		if (oldOneof == nil) != (newOneof == nil) {
			t.Errorf("%s: oneof membership changed", fieldPath)
		} else if oldOneof != nil && oldOneof.IsSynthetic() != newOneof.IsSynthetic() {
			t.Errorf("%s: optional presence encoding changed", fieldPath)
		}

		switch oldField.Kind() {
		case protoreflect.MessageKind, protoreflect.GroupKind:
			expectedName := relocatedDescriptorName(oldField.Message().FullName())
			if newField.Message().FullName() != expectedName {
				t.Errorf("%s: message type changed from %s to %s; expected %s", fieldPath, oldField.Message().FullName(), newField.Message().FullName(), expectedName)
				continue
			}
			compareMessageDescriptors(t, oldField.Message(), newField.Message(), seen)
		case protoreflect.EnumKind:
			compareEnums(t, fieldPath, oldField.Enum(), newField.Enum())
		}
	}
}

func relocatedDescriptorName(oldName protoreflect.FullName) protoreflect.FullName {
	if !strings.HasPrefix(string(oldName), "chatto.core.v1.") {
		return oldName
	}
	return relocatedName(oldName)
}

func compareEnums(t *testing.T, fieldPath string, oldEnum, newEnum protoreflect.EnumDescriptor) {
	t.Helper()
	expectedName := relocatedDescriptorName(oldEnum.FullName())
	if newEnum.FullName() != expectedName {
		t.Errorf("%s: enum type changed from %s to %s; expected %s", fieldPath, oldEnum.FullName(), newEnum.FullName(), expectedName)
		return
	}
	oldValues := oldEnum.Values()
	for i := 0; i < oldValues.Len(); i++ {
		oldValue := oldValues.Get(i)
		if newEnum.Values().ByNumber(oldValue.Number()) == nil {
			t.Errorf("%s: enum number %d was removed", fieldPath, oldValue.Number())
		}
	}
}

func verifyRepresentativePayloads(t *testing.T, oldDescriptor, newDescriptor protoreflect.MessageDescriptor) {
	t.Helper()
	selections := rootSelections(oldDescriptor)
	for _, selection := range selections {
		oldMessage := dynamicpb.NewMessage(oldDescriptor)
		populateMessage(oldMessage.ProtoReflect(), selection, 0, map[protoreflect.FullName]bool{})

		oldBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(oldMessage)
		if err != nil {
			t.Fatalf("marshal old %s: %v", oldDescriptor.FullName(), err)
		}
		newType, err := protoregistry.GlobalTypes.FindMessageByName(newDescriptor.FullName())
		if err != nil {
			t.Fatalf("find generated type %s: %v", newDescriptor.FullName(), err)
		}
		newMessage := newType.New().Interface()
		if err := proto.Unmarshal(oldBytes, newMessage); err != nil {
			t.Fatalf("decode old %s payload as %s: %v", oldDescriptor.FullName(), newDescriptor.FullName(), err)
		}
		assertNoUnknownFields(t, newMessage.ProtoReflect(), string(newDescriptor.FullName()))

		newBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(newMessage)
		if err != nil {
			t.Fatalf("marshal current %s: %v", newDescriptor.FullName(), err)
		}
		if !bytes.Equal(oldBytes, newBytes) {
			t.Errorf("%s selection %d changed its stored wire encoding", oldDescriptor.FullName(), selection)
		}
	}
}

func rootSelections(message protoreflect.MessageDescriptor) []protoreflect.FieldNumber {
	selections := []protoreflect.FieldNumber{0}
	oneofs := message.Oneofs()
	for i := 0; i < oneofs.Len(); i++ {
		oneof := oneofs.Get(i)
		if oneof.IsSynthetic() {
			continue
		}
		selections = selections[:0]
		fields := oneof.Fields()
		for j := 0; j < fields.Len(); j++ {
			selections = append(selections, fields.Get(j).Number())
		}
	}
	return selections
}

func populateMessage(message protoreflect.Message, rootSelection protoreflect.FieldNumber, depth int, stack map[protoreflect.FullName]bool) {
	if depth > 12 || stack[message.Descriptor().FullName()] {
		return
	}
	stack[message.Descriptor().FullName()] = true
	defer delete(stack, message.Descriptor().FullName())

	selectedOneofFields := map[protoreflect.FullName]protoreflect.FieldNumber{}
	oneofs := message.Descriptor().Oneofs()
	for i := 0; i < oneofs.Len(); i++ {
		oneof := oneofs.Get(i)
		if oneof.IsSynthetic() {
			continue
		}
		selected := oneof.Fields().Get(0).Number()
		if rootSelection != 0 && oneof.Fields().ByNumber(rootSelection) != nil {
			selected = rootSelection
		}
		selectedOneofFields[oneof.FullName()] = selected
	}

	fields := message.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if oneof := field.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() && selectedOneofFields[oneof.FullName()] != field.Number() {
			continue
		}
		if field.IsMap() {
			mapValue := message.Mutable(field).Map()
			mapValue.Set(sampleScalar(field.MapKey()).MapKey(), sampleFieldValue(field.MapValue(), depth+1, stack))
			continue
		}
		if field.IsList() {
			message.Mutable(field).List().Append(sampleFieldValue(field, depth+1, stack))
			continue
		}
		value := sampleFieldValue(field, depth+1, stack)
		if value.IsValid() {
			message.Set(field, value)
		}
	}
}

func sampleFieldValue(field protoreflect.FieldDescriptor, depth int, stack map[protoreflect.FullName]bool) protoreflect.Value {
	switch field.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		if stack[field.Message().FullName()] {
			return protoreflect.Value{}
		}
		message := dynamicpb.NewMessage(field.Message())
		populateMessage(message.ProtoReflect(), 0, depth, stack)
		return protoreflect.ValueOfMessage(message)
	default:
		return sampleScalar(field)
	}
}

func sampleScalar(field protoreflect.FieldDescriptor) protoreflect.Value {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true)
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		selected := values.Get(0).Number()
		for i := 0; i < values.Len(); i++ {
			if values.Get(i).Number() != 0 {
				selected = values.Get(i).Number()
				break
			}
		}
		return protoreflect.ValueOfEnum(selected)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(17)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(23)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(29)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(31)
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(3.5)
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(7.25)
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("compatibility-value")
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte("compatibility-bytes"))
	default:
		return protoreflect.Value{}
	}
}

func assertNoUnknownFields(t *testing.T, message protoreflect.Message, path string) {
	t.Helper()
	if len(message.GetUnknown()) > 0 {
		t.Errorf("%s contains unknown fields after decoding the old payload", path)
	}
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind && field.Kind() != protoreflect.GroupKind {
			return true
		}
		if field.IsMap() {
			if field.MapValue().Kind() == protoreflect.MessageKind {
				value.Map().Range(func(key protoreflect.MapKey, mapValue protoreflect.Value) bool {
					assertNoUnknownFields(t, mapValue.Message(), path+"."+string(field.Name()))
					return true
				})
			}
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				assertNoUnknownFields(t, list.Get(i).Message(), path+"."+string(field.Name()))
			}
			return true
		}
		assertNoUnknownFields(t, value.Message(), path+"."+string(field.Name()))
		return true
	})
}
