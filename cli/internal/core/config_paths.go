package core

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ConfigPath is the typed Go surface for one dynamic configuration path. The
// EVT payload stores ConfigValue, but callers use Path[T] helpers so value type
// checks stay at compile time.
type ConfigPath[T any] struct {
	Name      string
	Encode    func(T) (*configv1.ConfigValue, error)
	Decode    func(*configv1.ConfigValue) (T, error)
	Default   func(subject string) (T, bool)
	Validate  func(subject string, value T) error
	Authorize func(ctx context.Context, actorID string, subject string) error
}

func (p ConfigPath[T]) encode(subject string, value T) (*configv1.ConfigValue, error) {
	if err := validateConfigPath(p.Name); err != nil {
		return nil, err
	}
	if p.Validate != nil {
		if err := p.Validate(subject, value); err != nil {
			return nil, err
		}
	}
	if p.Encode == nil {
		return nil, fmt.Errorf("config path %q has no encoder", p.Name)
	}
	encoded, err := p.Encode(value)
	if err != nil {
		return nil, err
	}
	if encoded == nil {
		return nil, fmt.Errorf("config path %q encoded nil value", p.Name)
	}
	return encoded, nil
}

func stringConfigPath(name string) ConfigPath[string] {
	return ConfigPath[string]{
		Name: name,
		Encode: func(value string) (*configv1.ConfigValue, error) {
			return configStringValue(value), nil
		},
		Decode: func(value *configv1.ConfigValue) (string, error) {
			if value == nil {
				return "", nil
			}
			if _, ok := value.GetValue().(*configv1.ConfigValue_StringValue); !ok {
				return "", fmt.Errorf("config path %q expected string value", name)
			}
			return value.GetStringValue(), nil
		},
	}
}

func notificationLevelConfigPath(name string) ConfigPath[corev1.NotificationLevel] {
	return ConfigPath[corev1.NotificationLevel]{
		Name: name,
		Encode: func(value corev1.NotificationLevel) (*configv1.ConfigValue, error) {
			return configIntValue(int64(value)), nil
		},
		Decode: func(value *configv1.ConfigValue) (corev1.NotificationLevel, error) {
			if value == nil {
				return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, nil
			}
			if _, ok := value.GetValue().(*configv1.ConfigValue_IntValue); !ok {
				return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, fmt.Errorf("config path %q expected int value", name)
			}
			return corev1.NotificationLevel(value.GetIntValue()), nil
		},
	}
}

func timeFormatConfigPath(name string) ConfigPath[corev1.TimeFormat] {
	return ConfigPath[corev1.TimeFormat]{
		Name: name,
		Encode: func(value corev1.TimeFormat) (*configv1.ConfigValue, error) {
			return configIntValue(int64(value)), nil
		},
		Decode: func(value *configv1.ConfigValue) (corev1.TimeFormat, error) {
			if value == nil {
				return corev1.TimeFormat_TIME_FORMAT_UNSPECIFIED, nil
			}
			if _, ok := value.GetValue().(*configv1.ConfigValue_IntValue); !ok {
				return corev1.TimeFormat_TIME_FORMAT_UNSPECIFIED, fmt.Errorf("config path %q expected int value", name)
			}
			return corev1.TimeFormat(value.GetIntValue()), nil
		},
	}
}

func deprecatedAssetConfigPath(name string) ConfigPath[*corev1.DeprecatedAsset] {
	return ConfigPath[*corev1.DeprecatedAsset]{
		Name: name,
		Encode: func(value *corev1.DeprecatedAsset) (*configv1.ConfigValue, error) {
			if value == nil {
				return nil, fmt.Errorf("config path %q cannot store nil asset", name)
			}
			data, err := proto.Marshal(value)
			if err != nil {
				return nil, err
			}
			return configBytesValue(data), nil
		},
		Decode: func(value *configv1.ConfigValue) (*corev1.DeprecatedAsset, error) {
			if value == nil {
				return nil, nil
			}
			bytesValue, ok := value.GetValue().(*configv1.ConfigValue_BytesValue)
			if !ok {
				return nil, fmt.Errorf("config path %q expected bytes value", name)
			}
			if len(bytesValue.BytesValue) == 0 {
				return nil, nil
			}
			asset := &corev1.DeprecatedAsset{}
			if err := proto.Unmarshal(bytesValue.BytesValue, asset); err != nil {
				return nil, err
			}
			return asset, nil
		},
	}
}

func configStringValue(value string) *configv1.ConfigValue {
	return &configv1.ConfigValue{
		Value: &configv1.ConfigValue_StringValue{StringValue: value},
	}
}

func configIntValue(value int64) *configv1.ConfigValue {
	return &configv1.ConfigValue{
		Value: &configv1.ConfigValue_IntValue{IntValue: value},
	}
}

func configBytesValue(value []byte) *configv1.ConfigValue {
	return &configv1.ConfigValue{
		Value: &configv1.ConfigValue_BytesValue{BytesValue: value},
	}
}

var (
	ConfigPathServerName = stringConfigPath("server.name")

	ConfigPathServerDescription = stringConfigPath("server.description")

	ConfigPathServerWelcomeMessage = stringConfigPath("server.welcome_message")

	ConfigPathServerMOTD = stringConfigPath("server.motd")

	ConfigPathBlockedUsernames = ConfigPath[string]{
		Name: ConfigPathBlockedUsernamesName,
		Encode: func(value string) (*configv1.ConfigValue, error) {
			return configStringValue(value), nil
		},
		Decode: stringConfigPath(ConfigPathBlockedUsernamesName).Decode,
		Default: func(_ string) (string, bool) {
			return DefaultBlockedUsernames, true
		},
	}

	ConfigPathServerLogo = deprecatedAssetConfigPath("server.logo")

	ConfigPathServerBanner = deprecatedAssetConfigPath("server.banner")

	ConfigPathNotificationServerLevel = notificationLevelConfigPath("notifications.server.level")

	ConfigPathUserTimezone = stringConfigPath("preferences.timezone")

	ConfigPathUserTimeFormat = timeFormatConfigPath("preferences.time_format")
)

const ConfigPathBlockedUsernamesName = "auth.blocked_usernames"

func ConfigPathNotificationRoomLevel(roomID string) ConfigPath[corev1.NotificationLevel] {
	return notificationLevelConfigPath("notifications.rooms." + roomID + ".level")
}

func isNotificationRoomLevelPath(path string) bool {
	return strings.HasPrefix(path, "notifications.rooms.") && strings.HasSuffix(path, ".level")
}
