package assets

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"

	"github.com/gen2brain/heic"
)

func init() {
	// HEIF can declare a generic container brand. The decoder still checks
	// that the primary image uses a supported HEVC codec.
	image.RegisterFormat("heic", "????ftypmif1", heic.Decode, heic.DecodeConfig)
}

// validateHEICStill rejects sequence containers before pixel decoding. The
// decoder reads all sequence frames, which would bypass our still-image pixel
// budget. Walk only top-level box headers and seek over their payloads.
func validateHEICStill(r io.ReadSeeker) error {
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	for offset := int64(0); offset < end; {
		if _, err := r.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		var header [16]byte
		if _, err := io.ReadFull(r, header[:8]); err != nil {
			return fmt.Errorf("invalid HEIC box: %w", err)
		}
		size := uint64(binary.BigEndian.Uint32(header[:4]))
		minimum := uint64(8)
		if size == 1 {
			if _, err := io.ReadFull(r, header[8:]); err != nil {
				return fmt.Errorf("invalid HEIC box: %w", err)
			}
			size = binary.BigEndian.Uint64(header[8:])
			minimum = 16
		} else if size == 0 {
			size = uint64(end - offset)
		}
		if size < minimum || size > uint64(end-offset) {
			return fmt.Errorf("invalid HEIC box size")
		}
		if string(header[4:8]) == "moov" {
			return fmt.Errorf("HEIC image sequences are not supported")
		}
		offset += int64(size)
	}
	return nil
}
