package connectapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/Masterminds/semver"
	discoveryv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/discovery/v1"
	"golang.org/x/text/language"
)

// ListFeatures keeps the growing catalog out of bootstrap discovery. Filters
// affect only the response; they never change connection or permission policy.
func (s *serverDiscoveryService) ListFeatures(ctx context.Context, req *connect.Request[discoveryv1.ListFeaturesRequest]) (*connect.Response[discoveryv1.ListFeaturesResponse], error) {
	var after *semver.Version
	if req.Msg.IntroducedAfterFrontendVersion != nil {
		value := req.Msg.GetIntroducedAfterFrontendVersion()
		parsed, err := parseFeatureRelease(value)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("introduced_after_frontend_version must be a full Semantic Version"))
		}
		after = parsed
	}
	response := &discoveryv1.ListFeaturesResponse{}
	for _, feature := range serverFeatureCatalog(req.Msg.GetLocale()) {
		if after != nil {
			introduced, err := semver.NewVersion(feature.FrontendIntroducedVersion)
			if err != nil {
				return nil, connectInternalError(fmt.Errorf("invalid feature release: %w", err))
			}
			if !introduced.GreaterThan(after) {
				continue
			}
		}
		response.Features = append(response.Features, feature)
	}
	if callInfo, ok := connect.CallInfoForHandlerContext(ctx); ok && callInfo.HTTPMethod() == http.MethodGet {
		etag, err := discoveryResponseETag(response)
		if err != nil {
			return nil, connectInternalError(err)
		}
		headers := http.Header{"Cache-Control": []string{discoveryCacheControl}, "Etag": []string{etag}}
		if ifNoneMatch(callInfo.RequestHeader().Get("If-None-Match"), etag) {
			return nil, connect.NewNotModifiedError(headers)
		}
		maps.Copy(callInfo.ResponseHeader(), headers)
	}
	return connect.NewResponse(response), nil
}

// The pinned semver parser accepts abbreviated releases and numeric prerelease
// identifiers with leading zeroes. The public filter requires strict SemVer.
func parseFeatureRelease(value string) (*semver.Version, error) {
	parsed, err := semver.NewVersion(value)
	if err != nil {
		return nil, err
	}
	if parsed.String() != value {
		return nil, errors.New("non-canonical version")
	}
	for _, part := range strings.Split(parsed.Prerelease(), ".") {
		if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
			return nil, errors.New("numeric prerelease identifier has a leading zero")
		}
	}
	return parsed, nil
}

// Feature translations are static release data. English fallback text lives
// beside the feature IDs and introduction versions in serverFeatureCatalog.
//
//go:embed feature_catalog.json
var featureCatalogJSON []byte

type featureText struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var featureTranslations = func() map[string]map[string]featureText {
	var translations map[string]map[string]featureText
	if err := json.Unmarshal(featureCatalogJSON, &translations); err != nil {
		panic("invalid embedded feature translations: " + err.Error())
	}
	return translations
}()

// featureTranslation falls back per feature, so a partial new translation
// cannot remove an English description. Invalid tags also use English.
func featureTranslation(locale, id string) (featureText, bool) {
	tag, err := language.Parse(locale)
	if err != nil {
		return featureText{}, false
	}
	for tag != language.Und {
		if text, ok := featureTranslations[tag.String()][id]; ok && text.Name != "" && text.Description != "" {
			return text, true
		}
		tag = tag.Parent()
	}
	return featureText{}, false
}
