package config

import (
	"errors"
	"testing"

	"github.com/c2h5oh/datasize"
	"github.com/pelletier/go-toml/v2"
)

func TestByteSizePreservesDatasizeSemantics(t *testing.T) {
	for _, input := range []string{
		"0", "150", "25 MB", "25MB", "150mb", "1GB", "18446744073709551615",
		"", "-1", "1.5MB", "150Mb", "150Mib", "invalid", " 150MB",
		"18446744073709551616", "16EB",
	} {
		t.Run(input, func(t *testing.T) {
			var size ByteSize
			var legacy datasize.ByteSize
			err := size.UnmarshalText([]byte(input))
			legacyErr := legacy.UnmarshalText([]byte(input))
			if (err == nil) != (legacyErr == nil) || datasize.ByteSize(size) != legacy {
				t.Fatalf("decode = %d, %v; want %d, %v", size, err, legacy, legacyErr)
			}
			if legacyErr != nil && err.Error() != legacyErr.Error() {
				t.Fatalf("error = %v, want %v", err, legacyErr)
			}
			if !errors.Is(err, datasize.ErrBits) && errors.Is(legacyErr, datasize.ErrBits) {
				t.Fatal("lost bits error")
			}
		})
	}
}

func TestByteSizeTOMLRoundTrip(t *testing.T) {
	var cfg AssetsConfig
	if err := toml.Unmarshal([]byte(`max_upload_size = "150MiB"`), &cfg); err != nil {
		t.Fatal(err)
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AssetsConfig
	if err := toml.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.MaxUploadSize != 150<<20 {
		t.Fatalf("round-trip size = %d, want %d", decoded.MaxUploadSize, 150<<20)
	}
}
