package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// applyAuthProviderEnv owns the manually indexed provider environment format
// and its precedence over the deprecated single-provider OIDC variables. Keep
// this compatibility translation at the load boundary so the rest of the
// package only sees the canonical AuthConfig.Providers representation.
func applyAuthProviderEnv(cfg *ChattoConfig) error {
	providers, providersSet, err := authProvidersFromEnv()
	if err != nil {
		return err
	}
	legacyOIDCEnabled := strings.TrimSpace(os.Getenv("CHATTO_AUTH_OIDC_ENABLED"))

	if providersSet {
		if legacyOIDCEnabled != "" {
			return fmt.Errorf("CHATTO_AUTH_PROVIDERS_* cannot be combined with legacy CHATTO_AUTH_OIDC_ENABLED")
		}
		cfg.Auth.Providers = providers
		return nil
	}

	if legacyOIDCEnabled == "" {
		return nil
	}
	enabled, err := strconv.ParseBool(legacyOIDCEnabled)
	if err != nil {
		return fmt.Errorf("CHATTO_AUTH_OIDC_ENABLED must be a boolean: %w", err)
	}
	if !enabled {
		cfg.Auth.Providers = nil
		return nil
	}
	label := os.Getenv("CHATTO_AUTH_OIDC_LABEL")
	if label == "" {
		label = "Chatto Hub"
	}
	cfg.Auth.Providers = []AuthProviderConfig{{
		ID:           "oidc",
		Type:         AuthProviderTypeOpenIDConnect,
		Label:        label,
		IssuerURL:    os.Getenv("CHATTO_AUTH_OIDC_ISSUER_URL"),
		ClientID:     os.Getenv("CHATTO_AUTH_OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("CHATTO_AUTH_OIDC_CLIENT_SECRET"),
	}}
	return nil
}

func authProvidersFromEnv() ([]AuthProviderConfig, bool, error) {
	const prefix = "CHATTO_AUTH_PROVIDERS_"
	providersByIndex := make(map[int]*AuthProviderConfig)

	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, prefix) {
			continue
		}

		rest := strings.TrimPrefix(name, prefix)
		indexPart, field, ok := strings.Cut(rest, "_")
		if !ok {
			return nil, false, fmt.Errorf("%s must use CHATTO_AUTH_PROVIDERS_<index>_<field>", name)
		}
		index, err := strconv.Atoi(indexPart)
		if err != nil || index < 0 {
			return nil, false, fmt.Errorf("%s uses invalid provider index %q", name, indexPart)
		}

		provider := providersByIndex[index]
		if provider == nil {
			provider = &AuthProviderConfig{}
			providersByIndex[index] = provider
		}
		if err := applyAuthProviderEnvField(provider, name, field, value); err != nil {
			return nil, false, err
		}
	}

	if len(providersByIndex) == 0 {
		return nil, false, nil
	}

	indices := make([]int, 0, len(providersByIndex))
	for index := range providersByIndex {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for expected, index := range indices {
		if index != expected {
			return nil, false, fmt.Errorf("CHATTO_AUTH_PROVIDERS_* indexes must be contiguous starting at 0; missing index %d", expected)
		}
	}

	providers := make([]AuthProviderConfig, 0, len(indices))
	for _, index := range indices {
		providers = append(providers, *providersByIndex[index])
	}
	return providers, true, nil
}

func applyAuthProviderEnvField(provider *AuthProviderConfig, name, field, value string) error {
	switch field {
	case "ID":
		provider.ID = value
	case "TYPE":
		provider.Type = value
	case "LABEL":
		provider.Label = value
	case "CLIENT_ID":
		provider.ClientID = value
	case "CLIENT_SECRET":
		provider.ClientSecret = value
	case "ISSUER_URL":
		provider.IssuerURL = value
	case "SCOPES":
		provider.Scopes = splitCommaSeparatedEnv(value)
	case "REQUEST_EMAIL":
		requestEmail, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean: %w", name, err)
		}
		provider.RequestEmail = &requestEmail
	case "AUTO_PROVISION":
		autoProvision, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean: %w", name, err)
		}
		provider.AutoProvision = &autoProvision
	default:
		const providerOptionsPrefix = "PROVIDER_OPTIONS_"
		if strings.HasPrefix(field, providerOptionsPrefix) {
			optionName := strings.ToLower(strings.TrimPrefix(field, providerOptionsPrefix))
			if optionName == "" {
				return fmt.Errorf("%s must include a provider option name", name)
			}
			if provider.ProviderOptions == nil {
				provider.ProviderOptions = make(map[string]string)
			}
			provider.ProviderOptions[optionName] = value
			return nil
		}
		return fmt.Errorf("%s uses unknown auth provider field %q", name, field)
	}
	return nil
}

func splitCommaSeparatedEnv(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
