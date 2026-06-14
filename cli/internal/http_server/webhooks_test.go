package http_server

import "testing"

func TestLiveKitWebhookRoomBelongsToInstance(t *testing.T) {
	tests := []struct {
		name       string
		roomName   string
		instanceID string
		want       bool
	}{
		{
			name:       "matching hosted instance prefix",
			roomName:   "foo.channel_room",
			instanceID: "foo",
			want:       true,
		},
		{
			name:       "foreign hosted instance prefix",
			roomName:   "bar.channel_room",
			instanceID: "foo",
			want:       false,
		},
		{
			name:       "unprefixed room rejected for hosted instance",
			roomName:   "channel_room",
			instanceID: "foo",
			want:       false,
		},
		{
			name:       "legacy unprefixed room accepted without instance ID",
			roomName:   "channel_room",
			instanceID: "",
			want:       true,
		},
		{
			name:       "prefixed room rejected without instance ID",
			roomName:   "foo.channel_room",
			instanceID: "",
			want:       false,
		},
		{
			name:       "prefix must match exactly",
			roomName:   "foobar.channel_room",
			instanceID: "foo",
			want:       false,
		},
		{
			name:       "empty room rejected for hosted instance",
			roomName:   "",
			instanceID: "foo",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := liveKitWebhookRoomBelongsToInstance(tt.roomName, tt.instanceID)
			if got != tt.want {
				t.Fatalf("liveKitWebhookRoomBelongsToInstance(%q, %q) = %v, want %v", tt.roomName, tt.instanceID, got, tt.want)
			}
		})
	}
}
