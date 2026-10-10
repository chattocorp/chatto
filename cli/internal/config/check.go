package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CheckConfig checks names before running the normal configuration loader. It
// makes no network connections. Deprecated aliases fail this opt-in check, but
// remain accepted by ReadConfig. Diagnostics never include configuration values.
// An explicit path must exist; an absent default file permits ENV-only checks.
func CheckConfig(path string) error {
	explicit := path != ""
	if !explicit {
		path = "chatto.toml"
	}
	data, err := os.ReadFile(path)
	if err != nil && (explicit || !errors.Is(err, os.ErrNotExist)) {
		return errors.New("cannot read configuration file; check the path and file permissions")
	}
	var document map[string]any
	if err := toml.Unmarshal(data, &document); err != nil {
		// Parser errors can contain source snippets and secret values.
		return errors.New("invalid TOML syntax; check the configuration file (values omitted)")
	}
	var diagnostics []string
	checkTOMLNames(document, reflect.TypeFor[ChattoConfig](), "", "", &diagnostics)
	diagnostics = append(diagnostics, checkEnvNames(os.Environ())...)
	sort.Strings(diagnostics)
	if len(diagnostics) > 0 {
		return errors.New(strings.Join(diagnostics, "\n"))
	}
	if _, err := ReadConfig(path); err != nil {
		// Both decoder and validation errors can contain credentials, URLs, or PII.
		return errors.New("configuration values failed decoding or validation; check required settings, types, ranges, and provider indexes against the configuration reference (values omitted)")
	}
	return nil
}

// Retired settings are listed even when absent from the schema so operators get
// migration instructions instead of a generic unknown-name message.
var deprecatedTOML = map[string]string{
	"livekit.instance_id":               "use livekit.server_id",
	"livekit.webhook_url":               "remove this unused setting; configure webhook.urls in LiveKit",
	"auth.providers[].provider_options": "remove this unused setting; use documented auth.providers fields",
	"bootstrap.instance":                "use bootstrap.server",
	"bootstrap.users[].instance_role":   "use bootstrap.users[].server_role",
	"operator_api.socket_mode":          "remove this setting; the socket always uses mode 0600",
}

func configFields(t reflect.Type) map[string]reflect.StructField {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	fields := map[string]reflect.StructField{}
	if t.Kind() != reflect.Struct {
		return fields
	}
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if field.IsExported() && name != "" && name != "-" {
			fields[name] = field
		}
	}
	return fields
}

// schemaPath uses [] for array entries; displayPath keeps their numeric index.
func checkTOMLNames(document map[string]any, t reflect.Type, schemaPath, displayPath string, diagnostics *[]string) {
	fields := configFields(t)
	for name, value := range document {
		schemaKey, displayKey := schemaPath+name, displayPath+name
		if replacement, ok := deprecatedTOML[schemaKey]; ok {
			*diagnostics = append(*diagnostics, fmt.Sprintf("TOML %q is deprecated: %s", displayKey, replacement))
		}
		field, ok := fields[name]
		if !ok {
			if _, deprecated := deprecatedTOML[schemaKey]; !deprecated {
				*diagnostics = append(*diagnostics, fmt.Sprintf("unknown TOML key %q; remove it or use a documented configuration key", displayKey))
			}
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		switch v := value.(type) {
		case map[string]any:
			checkTOMLNames(v, fieldType, schemaKey+".", displayKey+".", diagnostics)
		case []any:
			if fieldType.Kind() != reflect.Slice {
				continue
			}
			for i, item := range v {
				if table, ok := item.(map[string]any); ok {
					checkTOMLNames(table, fieldType.Elem(), schemaKey+"[].", fmt.Sprintf("%s[%d].", displayKey, i), diagnostics)
				}
			}
		}
	}
}

func taggedEnvNames(t reflect.Type, names map[string]bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for field := range t.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("env"), ",")
		if name == "-" {
			continue
		}
		if name != "" {
			names[name] = true
		}
		taggedEnvNames(field.Type, names)
	}
}

func checkEnvNames(environ []string) []string {
	names := map[string]bool{"CHATTO_BOOTSTRAP_SERVER_NAME": true, "CHATTO_BOOTSTRAP_SERVER_ROOMS": true}
	taggedEnvNames(reflect.TypeFor[ChattoConfig](), names)
	deprecated := map[string]string{
		"CHATTO_LIVEKIT_INSTANCE_ID":      "use CHATTO_LIVEKIT_SERVER_ID",
		"CHATTO_LIVEKIT_WEBHOOK_URL":      "remove this unused setting; configure webhook.urls in LiveKit",
		"CHATTO_OPERATOR_API_SOCKET_MODE": "remove this setting; the socket always uses mode 0600",
	}
	for _, field := range []string{"ENABLED", "LABEL", "ISSUER_URL", "CLIENT_ID", "CLIENT_SECRET"} {
		deprecated["CHATTO_AUTH_OIDC_"+field] = "use CHATTO_AUTH_PROVIDERS_<index>_<field>; indexes start at 0"
	}
	var diagnostics []string
	indices := map[string]map[int]bool{}
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "CHATTO_") {
			continue
		}
		if replacement, ok := deprecated[name]; ok {
			diagnostics = append(diagnostics, fmt.Sprintf("ENV %q is deprecated: %s", name, replacement))
			continue
		}
		if names[name] {
			continue
		}
		known := false
		for prefix, typ := range map[string]reflect.Type{
			"CHATTO_AUTH_PROVIDERS_":  reflect.TypeFor[AuthProviderConfig](),
			"CHATTO_BOOTSTRAP_USERS_": reflect.TypeFor[BootstrapUser](),
			"CHATTO_BOOTSTRAP_BOTS_":  reflect.TypeFor[BootstrapBot](),
		} {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			index, field, ok := strings.Cut(strings.TrimPrefix(name, prefix), "_")
			n, err := strconv.Atoi(index)
			if !ok || err != nil || n < 0 {
				break
			}
			if indices[prefix] == nil {
				indices[prefix] = map[int]bool{}
			}
			indices[prefix][n] = true
			// These fields are supported by the manual indexed loaders, not env tags.
			_, known = configFields(typ)[strings.ToLower(field)]
			known = known && field == strings.ToUpper(field)
			if prefix == "CHATTO_BOOTSTRAP_USERS_" && field == "INSTANCE_ROLE" {
				known = false
			}
			if prefix == "CHATTO_AUTH_PROVIDERS_" && field == "PROVIDER_OPTIONS" {
				diagnostics = append(diagnostics, fmt.Sprintf("ENV %q is deprecated: remove this unused setting; use documented provider fields", name))
				known = true
			}
			break
		}
		if !known {
			diagnostics = append(diagnostics, fmt.Sprintf("unknown ENV name %q; remove it or use a documented CHATTO_ variable (indexed fields use <index>_<field>)", name))
		}
	}
	for prefix, present := range indices {
		for i := 0; i < len(present); i++ {
			if !present[i] {
				diagnostics = append(diagnostics, fmt.Sprintf("ENV %s* indexes must be contiguous starting at 0; missing index %d", prefix, i))
				break
			}
		}
	}
	return diagnostics
}
