package connectapi

import (
	"context"
	"strings"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// updateMaskSpec limits masks to editable resource fields. Request identifiers
// and concurrency tokens are deliberately not maskable. These resources have
// no editable nested messages; notification overrides use their own container.
type updateMaskSpec struct {
	fields          string
	container       protoreflect.Name
	preserveAbsence bool
}

var updateMaskSpecs = map[protoreflect.FullName]updateMaskSpec{
	"chatto.api.v1.UpdateProfileRequest":                                     {fields: "display_name login bio"},
	"chatto.api.v1.UpdateSettingsRequest":                                    {fields: "timezone time_format share_timezone"},
	"chatto.api.v1.UpdateRoomRequest":                                        {fields: "name description universal slow_mode_seconds threading_mode"},
	"chatto.api.v1.UpdateMessageRequest":                                     {fields: "body also_send_to_channel"},
	"chatto.api.v1.UpdateBotOutboundWebhookRequest":                          {fields: "enabled url authorization"},
	"chatto.admin.v1.UpdateUserRequest":                                      {fields: "display_name login"},
	"chatto.admin.v1.UpdateServerConfigRequest":                              {fields: "server_name description motd welcome_message"},
	"chatto.admin.v1.UpdateBlockedUsernamesRequest":                          {fields: "blocked_usernames"},
	"chatto.admin.v1.UpdateNeighborRequest":                                  {fields: "origin"},
	"chatto.admin.v1.UpdateRoleRequest":                                      {fields: "display_name description pingable"},
	"chatto.admin.v1.UpdateRoomGroupRequest":                                 {fields: "name description"},
	"chatto.admin.v1.UpdateSidebarLinkRequest":                               {fields: "label url"},
	"chatto.admin.v1.UpdateOAuthClientPolicyRequest":                         {fields: "policy"},
	"chatto.api.v1.NotificationPolicyServiceUpdateNotificationPolicyRequest": {fields: "direct_messages direct_mentions replies role_mentions here_mentions all_mentions followed_threads followed_rooms reactions room_messages", container: "overrides", preserveAbsence: true},
}

// normalizeUpdateMask returns a detached request containing only selected
// editable values, and an explicit canonical mask. It does not read or merge
// stored resources: the domain command still owns authorization and concurrency.
// A selected absent scalar becomes an explicit default for existing sparse
// command inputs. Nullable notification overrides retain absence to mean Inherit.
func normalizeUpdateMask[T proto.Message](request T) (T, error) {
	result := proto.Clone(request).(T)
	if err := applyUpdateMask(result); err != nil {
		return request, err
	}
	if err := protovalidate.Validate(result); err != nil {
		return request, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return result, nil
}

func applyUpdateMask(request proto.Message) error {
	root := request.ProtoReflect()
	spec, ok := updateMaskSpecs[root.Descriptor().FullName()]
	if !ok {
		return nil
	}
	maskField := root.Descriptor().Fields().ByName("update_mask")
	var mask *fieldmaskpb.FieldMask
	if root.Has(maskField) {
		mask = root.Get(maskField).Message().Interface().(*fieldmaskpb.FieldMask)
	}
	values := root
	if spec.container != "" {
		values = root.Mutable(root.Descriptor().Fields().ByName(spec.container)).Message()
	}
	allowed := strings.Fields(spec.fields)
	selected := map[string]bool{}
	if mask == nil {
		for _, name := range allowed {
			if values.Has(values.Descriptor().Fields().ByName(protoreflect.Name(name))) {
				selected[name] = true
			}
		}
	} else if len(mask.Paths) == 1 && mask.Paths[0] == "*" {
		for _, name := range allowed {
			selected[name] = true
		}
	} else {
		for _, path := range mask.Paths {
			valid := false
			for _, name := range allowed {
				if path == name {
					valid = true
					break
				}
			}
			if !valid {
				return invalidArgument("update_mask contains an unsupported field")
			}
			selected[path] = true
		}
	}
	if len(selected) == 0 {
		return invalidArgument("update_mask must select at least one editable field")
	}
	canonical := &fieldmaskpb.FieldMask{}
	for _, name := range allowed {
		field := values.Descriptor().Fields().ByName(protoreflect.Name(name))
		if !selected[name] {
			values.Clear(field)
			continue
		}
		canonical.Paths = append(canonical.Paths, name)
		if !spec.preserveAbsence && !values.Has(field) && !field.IsList() && !field.IsMap() {
			values.Set(field, field.Default())
		}
	}
	root.Set(maskField, protoreflect.ValueOfMessage(canonical.ProtoReflect()))
	return nil
}

// updateMaskInterceptor removes unselected values before protobuf validation.
// Otherwise an invalid value outside the mask could reject an unrelated edit.
func updateMaskInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			if message, ok := request.Any().(proto.Message); ok {
				if err := applyUpdateMask(message); err != nil {
					return nil, err
				}
			}
			return next(ctx, request)
		}
	})
}
