package graph

import (
	"strings"

	"hmans.de/chatto/internal/graph/model"
)

func fitModeString(fit *model.FitMode) string {
	if fit == nil {
		return "cover"
	}
	return strings.ToLower(string(*fit))
}
