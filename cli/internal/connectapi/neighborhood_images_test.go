package connectapi

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/core/remoteimage"
	cachestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/cache_state/v1"
)

func TestNeighborhoodImageURL(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &serverDiscoveryService{api: env.api}
	const origin = "https://remote.example"

	for name, tc := range map[string]struct {
		image *cachestatev1.NeighborhoodImage
		want  string
	}{
		"absolute source": {
			image: &cachestatev1.NeighborhoodImage{SourceUrl: "https://cdn.example/logo.png"},
			want:  "https://cdn.example/logo.png",
		},
		// Records from older versions can hold the relative URL that the
		// remote server advertised, together with a legacy object name.
		"legacy relative source": {
			image: &cachestatev1.NeighborhoodImage{SourceUrl: "/assets/logo.png", ObjectName: "abc"},
			want:  "https://remote.example/assets/logo.png",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := service.neighborhoodImageURL(origin, tc.image, remoteimage.PresetAvatar)
			if got == nil || *got != env.core.RemoteImagePath(remoteimage.PresetAvatar, tc.want) {
				t.Fatalf("neighborhoodImageURL = %v, want path for %q", got, tc.want)
			}
		})
	}

	for name, image := range map[string]*cachestatev1.NeighborhoodImage{
		"no image":       nil,
		"empty source":   {},
		"invalid source": {SourceUrl: "ftp://remote.example/logo.png"},
	} {
		if got := service.neighborhoodImageURL(origin, image, remoteimage.PresetAvatar); got != nil {
			t.Errorf("%s: neighborhoodImageURL = %q, want nil", name, *got)
		}
	}
}
