package core

// ModelKeys returns the stable metric keys in the core model inventory.
func ModelKeys() []string {
	return []string{
		"chatto_core",
		"event_publisher",
		"config_model",
		"notification_preferences_model",
		"message_model",
		"reaction_model",
		"room_timeline_read_model",
		"read_state_model",
		"thread_follow_model",
		"room_model",
		"user_model",
		"rbac_model",
		"mentionables_model",
		"presence_model",
		"my_events_model",
		"call_model",
		"media_model",
		"asset_model",
	}
}
