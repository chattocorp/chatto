package rbac

import "testing"

func TestRoleKey(t *testing.T) {
	tests := []struct {
		name     string
		roleName string
		want     string
	}{
		{"admin role", "admin", "role.admin"},
		{"everyone role", "everyone", "role.everyone"},
		{"instance admin", "instance-admin", "role.instance-admin"},
		{"custom role", "moderator", "role.moderator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoleKey(tt.roleName); got != tt.want {
				t.Errorf("RoleKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemberKey(t *testing.T) {
	tests := []struct {
		name     string
		roleName string
		userID   string
		want     string
	}{
		{"admin assignment", "admin", "U9mP2qR5tYz3wK", "member.admin.U9mP2qR5tYz3wK"},
		{"instance admin", "instance-admin", "U9mP2qR5tYz3wK", "member.instance-admin.U9mP2qR5tYz3wK"},
		{"moderator", "moderator", "Uabc123def456x", "member.moderator.Uabc123def456x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MemberKey(tt.roleName, tt.userID); got != tt.want {
				t.Errorf("MemberKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllowKey(t *testing.T) {
	tests := []struct {
		name       string
		subject    string
		verb       string
		objectType string
		objectId   string
		want       string
	}{
		{"instance role grant", "instance-admin", "access", "admin", "any", "allow.instance-admin.access.admin.any"},
		{"space role grant", "admin", "create", "room", "any", "allow.admin.create.room.any"},
		{"user space grant", "U9mP2qR5tYz3wK", "create", "room", "any", "allow.U9mP2qR5tYz3wK.create.room.any"},
		{"room permission", "everyone", "post", "message", "Rabc456", "allow.everyone.post.message.Rabc456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowKey(tt.subject, tt.verb, tt.objectType, tt.objectId); got != tt.want {
				t.Errorf("AllowKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDenyKey(t *testing.T) {
	tests := []struct {
		name       string
		subject    string
		verb       string
		objectType string
		objectId   string
		want       string
	}{
		{"instance denial", "instance-admin", "create", "space", "any", "deny.instance-admin.create.space.any"},
		{"space role denial", "everyone", "create", "room", "any", "deny.everyone.create.room.any"},
		{"user space denial", "U9mP2qR5tYz3wK", "create", "room", "any", "deny.U9mP2qR5tYz3wK.create.room.any"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DenyKey(tt.subject, tt.verb, tt.objectType, tt.objectId); got != tt.want {
				t.Errorf("DenyKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemberPatternForRole(t *testing.T) {
	tests := []struct {
		name     string
		roleName string
		want     string
	}{
		{"admin", "admin", "member.admin.*"},
		{"instance-admin", "instance-admin", "member.instance-admin.*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MemberPatternForRole(tt.roleName); got != tt.want {
				t.Errorf("MemberPatternForRole() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemberPatternForUser(t *testing.T) {
	tests := []struct {
		name   string
		userID string
		want   string
	}{
		{"user", "U9mP2qR5tYz3wK", "member.*.U9mP2qR5tYz3wK"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MemberPatternForUser(tt.userID); got != tt.want {
				t.Errorf("MemberPatternForUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllowPatternForSubject(t *testing.T) {
	if got := AllowPatternForSubject("admin"); got != "allow.admin.>" {
		t.Errorf("AllowPatternForSubject() = %v, want allow.admin.>", got)
	}
}

func TestAllowPatternForSubjectVerb(t *testing.T) {
	if got := AllowPatternForSubjectVerb("admin", "create"); got != "allow.admin.create.>" {
		t.Errorf("AllowPatternForSubjectVerb() = %v, want allow.admin.create.>", got)
	}
}

func TestAllowPatternForSubjectVerbType(t *testing.T) {
	if got := AllowPatternForSubjectVerbType("admin", "create", "room"); got != "allow.admin.create.room.*" {
		t.Errorf("AllowPatternForSubjectVerbType() = %v, want allow.admin.create.room.*", got)
	}
}

func TestAllowPatternForObjectType(t *testing.T) {
	if got := AllowPatternForObjectType("room"); got != "allow.*.*.room.*" {
		t.Errorf("AllowPatternForObjectType() = %v, want allow.*.*.room.*", got)
	}
}

func TestDenyPatternForSubject(t *testing.T) {
	if got := DenyPatternForSubject("everyone"); got != "deny.everyone.>" {
		t.Errorf("DenyPatternForSubject() = %v, want deny.everyone.>", got)
	}
}

func TestDenyPatternForSubjectVerb(t *testing.T) {
	if got := DenyPatternForSubjectVerb("everyone", "create"); got != "deny.everyone.create.>" {
		t.Errorf("DenyPatternForSubjectVerb() = %v, want deny.everyone.create.>", got)
	}
}

func TestDenyPatternForSubjectVerbType(t *testing.T) {
	if got := DenyPatternForSubjectVerbType("everyone", "create", "room"); got != "deny.everyone.create.room.*" {
		t.Errorf("DenyPatternForSubjectVerbType() = %v, want deny.everyone.create.room.*", got)
	}
}

func TestDenyPatternForObjectType(t *testing.T) {
	if got := DenyPatternForObjectType("room"); got != "deny.*.*.room.*" {
		t.Errorf("DenyPatternForObjectType() = %v, want deny.*.*.room.*", got)
	}
}

func TestIsUserSubject(t *testing.T) {
	tests := []struct {
		subject string
		want    bool
	}{
		{"U9mP2qR5tYz3wK", true},
		{"Uabc123def456x", true},
		{"admin", false},
		{"everyone", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			if got := IsUserSubject(tt.subject); got != tt.want {
				t.Errorf("IsUserSubject(%q) = %v, want %v", tt.subject, got, tt.want)
			}
		})
	}
}

func TestIsRoleSubject(t *testing.T) {
	tests := []struct {
		subject string
		want    bool
	}{
		{"admin", true},
		{"everyone", true},
		{"moderator", true},
		{"U9mP2qR5tYz3wK", false},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			if got := IsRoleSubject(tt.subject); got != tt.want {
				t.Errorf("IsRoleSubject(%q) = %v, want %v", tt.subject, got, tt.want)
			}
		})
	}
}

func TestParseAllowKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want PermissionKeyParts
	}{
		{
			"valid allow key",
			"allow.admin.create.room.any",
			PermissionKeyParts{Subject: "admin", Verb: "create", ObjectType: "room", ObjectId: "any"},
		},
		{
			"allow key with room id",
			"allow.everyone.post.message.Rabc456",
			PermissionKeyParts{Subject: "everyone", Verb: "post", ObjectType: "message", ObjectId: "Rabc456"},
		},
		{
			"invalid prefix",
			"deny.admin.create.room.any",
			PermissionKeyParts{},
		},
		{
			"too few parts",
			"allow.admin.create",
			PermissionKeyParts{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAllowKey(tt.key)
			if got != tt.want {
				t.Errorf("ParseAllowKey(%q) = %+v, want %+v", tt.key, got, tt.want)
			}
		})
	}
}

func TestParseDenyKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want PermissionKeyParts
	}{
		{
			"valid deny key",
			"deny.everyone.create.room.any",
			PermissionKeyParts{Subject: "everyone", Verb: "create", ObjectType: "room", ObjectId: "any"},
		},
		{
			"invalid prefix",
			"allow.everyone.create.room.any",
			PermissionKeyParts{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDenyKey(tt.key)
			if got != tt.want {
				t.Errorf("ParseDenyKey(%q) = %+v, want %+v", tt.key, got, tt.want)
			}
		})
	}
}

func TestSetAllowKey(t *testing.T) {
	tests := []struct {
		name       string
		groupId      string
		subject    string
		verb       string
		objectType string
		want       string
	}{
		{"role grant", "Sgeneral", "moderator", "post", "message", "group_allow.Sgeneral.moderator.post.message"},
		{"everyone grant", "Sgeneral", "everyone", "post", "message", "group_allow.Sgeneral.everyone.post.message"},
		{"user grant", "Seng", "U9mP2qR5tYz3wK", "manage", "room", "group_allow.Seng.U9mP2qR5tYz3wK.manage.room"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GroupAllowKey(tt.groupId, tt.subject, tt.verb, tt.objectType); got != tt.want {
				t.Errorf("GroupAllowKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetDenyKey(t *testing.T) {
	if got := GroupDenyKey("Sgeneral", "everyone", "post", "message"); got != "group_deny.Sgeneral.everyone.post.message" {
		t.Errorf("GroupDenyKey() = %v, want group_deny.Sgeneral.everyone.post.message", got)
	}
}

func TestSetAllowPatternForSet(t *testing.T) {
	if got := GroupAllowPatternForGroup("Sgeneral"); got != "group_allow.Sgeneral.>" {
		t.Errorf("GroupAllowPatternForGroup() = %v, want group_allow.Sgeneral.>", got)
	}
}

func TestSetDenyPatternForSet(t *testing.T) {
	if got := GroupDenyPatternForGroup("Sgeneral"); got != "group_deny.Sgeneral.>" {
		t.Errorf("GroupDenyPatternForGroup() = %v, want group_deny.Sgeneral.>", got)
	}
}

func TestRoomAllowKey(t *testing.T) {
	tests := []struct {
		name       string
		roomId     string
		subject    string
		verb       string
		objectType string
		want       string
	}{
		{"announcements override", "Rannounce", "everyone", "post", "message", "room_allow.Rannounce.everyone.post.message"},
		{"per-user grant", "Rgeneral", "U9mP2qR5tYz3wK", "delete-any", "message", "room_allow.Rgeneral.U9mP2qR5tYz3wK.delete-any.message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoomAllowKey(tt.roomId, tt.subject, tt.verb, tt.objectType); got != tt.want {
				t.Errorf("RoomAllowKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRoomDenyKey(t *testing.T) {
	if got := RoomDenyKey("Rannounce", "everyone", "post", "message"); got != "room_deny.Rannounce.everyone.post.message" {
		t.Errorf("RoomDenyKey() = %v, want room_deny.Rannounce.everyone.post.message", got)
	}
}

func TestRoomAllowPatternForRoom(t *testing.T) {
	if got := RoomAllowPatternForRoom("Rgeneral"); got != "room_allow.Rgeneral.>" {
		t.Errorf("RoomAllowPatternForRoom() = %v, want room_allow.Rgeneral.>", got)
	}
}

func TestRoomDenyPatternForRoom(t *testing.T) {
	if got := RoomDenyPatternForRoom("Rgeneral"); got != "room_deny.Rgeneral.>" {
		t.Errorf("RoomDenyPatternForRoom() = %v, want room_deny.Rgeneral.>", got)
	}
}

func TestParseSetAllowKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want ScopedPermissionKeyParts
	}{
		{
			"valid set allow",
			"group_allow.Sgeneral.everyone.post.message",
			ScopedPermissionKeyParts{ScopeID: "Sgeneral", Subject: "everyone", Verb: "post", ObjectType: "message"},
		},
		{
			"user subject in set",
			"group_allow.Seng.U9mP2qR5tYz3wK.manage.room",
			ScopedPermissionKeyParts{ScopeID: "Seng", Subject: "U9mP2qR5tYz3wK", Verb: "manage", ObjectType: "room"},
		},
		{
			"wrong prefix",
			"group_deny.Sgeneral.everyone.post.message",
			ScopedPermissionKeyParts{},
		},
		{
			"too few parts",
			"group_allow.Sgeneral.everyone.post",
			ScopedPermissionKeyParts{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSetAllowKey(tt.key)
			if got != tt.want {
				t.Errorf("ParseSetAllowKey(%q) = %+v, want %+v", tt.key, got, tt.want)
			}
		})
	}
}

func TestParseSetDenyKey(t *testing.T) {
	got := ParseSetDenyKey("group_deny.Sgeneral.everyone.post.message")
	want := ScopedPermissionKeyParts{ScopeID: "Sgeneral", Subject: "everyone", Verb: "post", ObjectType: "message"}
	if got != want {
		t.Errorf("ParseSetDenyKey() = %+v, want %+v", got, want)
	}
}

func TestParseRoomAllowKey(t *testing.T) {
	got := ParseRoomAllowKey("room_allow.Rannounce.moderator.post.message")
	want := ScopedPermissionKeyParts{ScopeID: "Rannounce", Subject: "moderator", Verb: "post", ObjectType: "message"}
	if got != want {
		t.Errorf("ParseRoomAllowKey() = %+v, want %+v", got, want)
	}
}

func TestParseRoomDenyKey(t *testing.T) {
	got := ParseRoomDenyKey("room_deny.Rannounce.everyone.post.message")
	want := ScopedPermissionKeyParts{ScopeID: "Rannounce", Subject: "everyone", Verb: "post", ObjectType: "message"}
	if got != want {
		t.Errorf("ParseRoomDenyKey() = %+v, want %+v", got, want)
	}
}

func TestParseScopedKey_RejectsLegacyShape(t *testing.T) {
	// The legacy server-scope allow.X.Y.Z.W format must not match the scoped
	// parsers, even though the part counts coincide.
	if got := ParseSetAllowKey("allow.everyone.post.message.any"); got != (ScopedPermissionKeyParts{}) {
		t.Errorf("ParseSetAllowKey on legacy key = %+v, want zero", got)
	}
	if got := ParseRoomAllowKey("allow.everyone.post.message.Rabc"); got != (ScopedPermissionKeyParts{}) {
		t.Errorf("ParseRoomAllowKey on legacy key = %+v, want zero", got)
	}
}

func TestParseMemberKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		wantRole string
		wantUser string
	}{
		{
			"valid member key",
			"member.admin.U9mP2qR5tYz3wK",
			"admin",
			"U9mP2qR5tYz3wK",
		},
		{
			"instance role member key",
			"member.instance-admin.Uabc123",
			"instance-admin",
			"Uabc123",
		},
		{
			"invalid prefix",
			"role.admin.U9mP2qR5tYz3wK",
			"",
			"",
		},
		{
			"too few parts",
			"member.admin",
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRole, gotUser := ParseMemberKey(tt.key)
			if gotRole != tt.wantRole || gotUser != tt.wantUser {
				t.Errorf("ParseMemberKey(%q) = (%q, %q), want (%q, %q)", tt.key, gotRole, gotUser, tt.wantRole, tt.wantUser)
			}
		})
	}
}
