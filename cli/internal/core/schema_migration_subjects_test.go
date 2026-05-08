package core

import "testing"

func TestRewriteSubjectForServerStream(t *testing.T) {
	cases := []struct {
		name    string
		legacy  string
		kind    string
		want    string
		wantOK  bool
	}{
		{
			name:   "primary root message → channel kind",
			legacy: "space.Sprimary.room.Rroom.msg.Eevt",
			kind:   "channel",
			want:   "server.room.channel.Rroom.msg.Eevt",
			wantOK: true,
		},
		{
			name:   "DM root message → dm kind",
			legacy: "space.DM.room.Rroom.msg.Eevt",
			kind:   "dm",
			want:   "server.room.dm.Rroom.msg.Eevt",
			wantOK: true,
		},
		{
			name:   "primary thread reply",
			legacy: "space.Sprimary.room.Rroom.msg.Eroot.replies.Erep",
			kind:   "channel",
			want:   "server.room.channel.Rroom.msg.Eroot.replies.Erep",
			wantOK: true,
		},
		{
			name:   "DM thread reply",
			legacy: "space.DM.room.Rroom.msg.Eroot.replies.Erep",
			kind:   "dm",
			want:   "server.room.dm.Rroom.msg.Eroot.replies.Erep",
			wantOK: true,
		},
		{
			name:   "primary meta",
			legacy: "space.Sprimary.room.Rroom.meta",
			kind:   "channel",
			want:   "server.room.channel.Rroom.meta",
			wantOK: true,
		},
		{
			name:   "DM meta",
			legacy: "space.DM.room.Rroom.meta",
			kind:   "dm",
			want:   "server.room.dm.Rroom.meta",
			wantOK: true,
		},
		{
			name:   "membership: member_deleted strips prefix",
			legacy: "space.Sprimary.member_deleted",
			kind:   "channel",
			want:   "server.member.deleted",
			wantOK: true,
		},
		{
			name:   "membership: bare verb passes through",
			legacy: "space.Sprimary.joined",
			kind:   "channel",
			want:   "server.member.joined",
			wantOK: true,
		},
		{
			name:   "unknown shape: too few segments",
			legacy: "space.Sprimary",
			kind:   "channel",
			wantOK: false,
		},
		{
			name:   "unknown shape: garbage tail",
			legacy: "space.Sprimary.room.Rroom.unexpected",
			kind:   "channel",
			wantOK: false,
		},
		{
			name:   "unknown shape: wrong prefix",
			legacy: "server.room.channel.Rroom.msg.Eevt",
			kind:   "channel",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rewriteSubjectForServerStream(tc.legacy, tc.kind)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got=%q)", ok, tc.wantOK, got)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
