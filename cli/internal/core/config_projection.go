package core

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const ConfigSubjectServer = "server"

// ConfigProjection consumes dynamic configuration events from EVT and keeps the
// current latest-value map in memory. Legacy ServerConfigChangedEvent snapshots
// are also applied so old migration events remain readable while the generic
// config model rolls out.
type ConfigProjection struct {
	events.MemoryProjection
	values map[string]map[string]*configv1.ConfigValue
}

// ServerConfigProjection is kept as a compatibility alias while callers and
// tests move from the old singleton server-config projection name.
type ServerConfigProjection = ConfigProjection

func NewConfigProjection() *ConfigProjection {
	return &ConfigProjection{values: make(map[string]map[string]*configv1.ConfigValue)}
}

func NewServerConfigProjection() *ConfigProjection {
	return NewConfigProjection()
}

func (p *ConfigProjection) Subjects() []string {
	return []string{events.ConfigSubjectFilter()}
}

func (p *ConfigProjection) Apply(event *corev1.Event, _ uint64) error {
	if event == nil {
		return nil
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_ConfigValueSet:
		if e.ConfigValueSet == nil {
			return nil
		}
		return p.set(e.ConfigValueSet.GetSubject(), e.ConfigValueSet.GetPath(), e.ConfigValueSet.GetValue())
	case *corev1.Event_ConfigValueCleared:
		if e.ConfigValueCleared == nil {
			return nil
		}
		p.clear(e.ConfigValueCleared.GetSubject(), e.ConfigValueCleared.GetPath())
	case *corev1.Event_ServerConfigChanged:
		if e.ServerConfigChanged != nil {
			return p.applyLegacyServerConfig(e.ServerConfigChanged.GetConfig())
		}
	}
	return nil
}

func (p *ConfigProjection) set(subject, path string, value *configv1.ConfigValue) error {
	if subject == "" || path == "" || value == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if p.values == nil {
		p.values = make(map[string]map[string]*configv1.ConfigValue)
	}
	byPath := p.values[subject]
	if byPath == nil {
		byPath = make(map[string]*configv1.ConfigValue)
		p.values[subject] = byPath
	}
	byPath[path] = proto.Clone(value).(*configv1.ConfigValue)
	return nil
}

func (p *ConfigProjection) clear(subject, path string) {
	if subject == "" || path == "" {
		return
	}
	p.Lock()
	defer p.Unlock()
	if byPath := p.values[subject]; byPath != nil {
		delete(byPath, path)
		if len(byPath) == 0 {
			delete(p.values, subject)
		}
	}
}

func (p *ConfigProjection) Value(subject, path string) (*configv1.ConfigValue, bool) {
	p.RLock()
	defer p.RUnlock()
	byPath := p.values[subject]
	if byPath == nil {
		return nil, false
	}
	value := byPath[path]
	if value == nil {
		return nil, false
	}
	return proto.Clone(value).(*configv1.ConfigValue), true
}

func (p *ConfigProjection) SubjectConfigured(subject string) bool {
	p.RLock()
	defer p.RUnlock()
	return len(p.values[subject]) > 0
}

func (p *ConfigProjection) PathsWithPrefix(subject, prefix string) []string {
	p.RLock()
	defer p.RUnlock()
	byPath := p.values[subject]
	if len(byPath) == 0 {
		return nil
	}
	paths := make([]string, 0)
	for path := range byPath {
		if strings.HasPrefix(path, prefix) {
			paths = append(paths, path)
		}
	}
	return paths
}

func (p *ConfigProjection) applyLegacyServerConfig(cfg *configv1.ServerConfig) error {
	if cfg == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	if p.values == nil {
		p.values = make(map[string]map[string]*configv1.ConfigValue)
	}
	byPath := p.values[ConfigSubjectServer]
	if byPath == nil {
		byPath = make(map[string]*configv1.ConfigValue)
		p.values[ConfigSubjectServer] = byPath
	}
	byPath[ConfigPathServerName.Name] = configStringValue(cfg.GetServerName())
	byPath[ConfigPathServerDescription.Name] = configStringValue(cfg.GetDescription())
	byPath[ConfigPathServerWelcomeMessage.Name] = configStringValue(cfg.GetWelcomeMessage())
	byPath[ConfigPathServerMOTD.Name] = configStringValue(cfg.GetMotd())
	byPath[ConfigPathBlockedUsernames.Name] = configStringValue(cfg.GetBlockedUsernames())
	return nil
}

func (p *ConfigProjection) Get() (cfg *configv1.ServerConfig, isConfigured bool) {
	p.RLock()
	defer p.RUnlock()
	byPath := p.values[ConfigSubjectServer]
	if len(byPath) == 0 {
		return nil, false
	}
	return &configv1.ServerConfig{
		ServerName:       configStringFromMap(byPath, ConfigPathServerName.Name),
		Description:      configStringFromMap(byPath, ConfigPathServerDescription.Name),
		WelcomeMessage:   configStringFromMap(byPath, ConfigPathServerWelcomeMessage.Name),
		Motd:             configStringFromMap(byPath, ConfigPathServerMOTD.Name),
		BlockedUsernames: configStringFromMap(byPath, ConfigPathBlockedUsernames.Name),
	}, true
}

func (p *ConfigProjection) EffectiveServerName() string {
	if value, ok, _ := getProjectedConfig(p, ConfigSubjectServer, ConfigPathServerName); ok && value != "" {
		return value
	}
	return "Chatto"
}

func (p *ConfigProjection) EffectiveWelcomeMessage() string {
	value, _, _ := getProjectedConfig(p, ConfigSubjectServer, ConfigPathServerWelcomeMessage)
	return value
}

func (p *ConfigProjection) EffectiveMOTD() string {
	value, _, _ := getProjectedConfig(p, ConfigSubjectServer, ConfigPathServerMOTD)
	return value
}

func (p *ConfigProjection) EffectiveDescription() string {
	if value, ok, _ := getProjectedConfig(p, ConfigSubjectServer, ConfigPathServerDescription); ok && value != "" {
		return value
	}
	return DefaultDescription
}

func (p *ConfigProjection) EffectiveBlockedUsernames() string {
	value, ok, _ := getProjectedConfig(p, ConfigSubjectServer, ConfigPathBlockedUsernames)
	if !ok {
		return DefaultBlockedUsernames
	}
	return value
}

func (p *ConfigProjection) BlockedUsernamesList() []string {
	return parseBlockedUsernames(p.EffectiveBlockedUsernames())
}

func (p *ConfigProjection) IsUsernameBlocked(login string) bool {
	loginLower := strings.ToLower(login)
	for _, blocked := range p.BlockedUsernamesList() {
		if blocked == loginLower {
			return true
		}
	}
	return false
}

func (p *ConfigProjection) NotificationServerLevel(userID string) corev1.NotificationLevel {
	level, ok, _ := getProjectedConfig(p, userID, ConfigPathNotificationServerLevel)
	if !ok {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	}
	return level
}

func (p *ConfigProjection) NotificationRoomLevel(userID, roomID string) corev1.NotificationLevel {
	level, ok, _ := getProjectedConfig(p, userID, ConfigPathNotificationRoomLevel(roomID))
	if !ok {
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	}
	return level
}

func getProjectedConfig[T any](p *ConfigProjection, subject string, path ConfigPath[T]) (T, bool, error) {
	var zero T
	value, ok := p.Value(subject, path.Name)
	if !ok {
		if path.Default != nil {
			if def, hasDefault := path.Default(subject); hasDefault {
				return def, false, nil
			}
		}
		return zero, false, nil
	}
	decoded, err := path.Decode(value)
	if err != nil {
		return zero, true, err
	}
	return decoded, true, nil
}

func configStringFromMap(values map[string]*configv1.ConfigValue, path string) string {
	value := values[path]
	if value == nil {
		return ""
	}
	return value.GetStringValue()
}

func validateConfigSubject(subject string) error {
	if subject == "" {
		return fmt.Errorf("config subject is empty")
	}
	if strings.ContainsAny(subject, ". \t\r\n") || subject == "*" || subject == ">" {
		return fmt.Errorf("invalid config subject %q", subject)
	}
	return nil
}

func validateConfigPath(path string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}
	for _, token := range strings.Split(path, ".") {
		if token == "" || token == "*" || token == ">" || strings.ContainsAny(token, " \t\r\n") {
			return fmt.Errorf("invalid config path %q", path)
		}
	}
	return nil
}
