package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// snapshotContractID combines a manually managed restore-semantics version
// with the reachable protobuf schema. Schema changes therefore select a new
// snapshot namespace without requiring obsolete codec messages in the binary.
func snapshotContractID(semantics string, message proto.Message) string {
	fingerprint := snapshotSchemaFingerprint(message.ProtoReflect().Descriptor())
	return semantics + "-" + fingerprint
}

func snapshotSchemaFingerprint(root protoreflect.MessageDescriptor) string {
	var schema strings.Builder
	seenMessages := make(map[protoreflect.FullName]bool)
	seenEnums := make(map[protoreflect.FullName]bool)

	var writeEnum func(protoreflect.EnumDescriptor)
	writeEnum = func(enum protoreflect.EnumDescriptor) {
		if seenEnums[enum.FullName()] {
			return
		}
		seenEnums[enum.FullName()] = true
		fmt.Fprintf(&schema, "enum %s;", enum.FullName())
		values := make([]protoreflect.EnumValueDescriptor, enum.Values().Len())
		for i := range values {
			values[i] = enum.Values().Get(i)
		}
		sort.Slice(values, func(i, j int) bool {
			if values[i].Number() == values[j].Number() {
				return values[i].Name() < values[j].Name()
			}
			return values[i].Number() < values[j].Number()
		})
		for _, value := range values {
			fmt.Fprintf(&schema, "%d:%s;", value.Number(), value.Name())
		}
	}

	var writeMessage func(protoreflect.MessageDescriptor)
	writeMessage = func(message protoreflect.MessageDescriptor) {
		if seenMessages[message.FullName()] {
			return
		}
		seenMessages[message.FullName()] = true
		fmt.Fprintf(&schema, "message %s;", message.FullName())
		fields := make([]protoreflect.FieldDescriptor, message.Fields().Len())
		for i := range fields {
			fields[i] = message.Fields().Get(i)
		}
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].Number() < fields[j].Number()
		})
		for _, field := range fields {
			oneof := protoreflect.Name("")
			referencedType := protoreflect.FullName("")
			if field.ContainingOneof() != nil {
				oneof = field.ContainingOneof().Name()
			}
			switch field.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				referencedType = field.Message().FullName()
			case protoreflect.EnumKind:
				referencedType = field.Enum().FullName()
			}
			fmt.Fprintf(
				&schema,
				"%d:%s:%s:%s:%t:%t:%s:%s;",
				field.Number(),
				field.Name(),
				field.Cardinality(),
				field.Kind(),
				field.HasPresence(),
				field.IsMap(),
				oneof,
				referencedType,
			)
			switch field.Kind() {
			case protoreflect.MessageKind, protoreflect.GroupKind:
				writeMessage(field.Message())
			case protoreflect.EnumKind:
				writeEnum(field.Enum())
			}
		}
	}

	writeMessage(root)
	sum := sha256.Sum256([]byte(schema.String()))
	return hex.EncodeToString(sum[:8])
}
