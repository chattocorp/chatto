package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	op "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"io"
)

// messageAssets uploads on first use so unattached uploads do not age while
// earlier history is imported. A source file can be used by several messages.
func (m *cliArchiveImport) messageAssets(ctx context.Context, message, room string, ids []string) ([]string, error) {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		key := "asset/" + importRef(message) + "/" + importRef(id)
		native := m.progress.values[key]
		if native == "" {
			a := m.assets[id]
			session := m.progress.values[key+"/complete"]
			if session == "" {
				f, err := m.archive.OpenAsset(a)
				if err != nil {
					return nil, err
				}
				res, err := m.assetAPI.CreateUpload(ctx, operatorRequest(&op.CreateUploadRequest{RoomId: room, AuthorId: m.users[m.authors[message]], Filename: a.Filename, ContentType: a.MediaType, Size: a.Storage.Size, Sha256: a.Storage.SHA256, Historical: true}))
				if err != nil {
					f.Close()
					return nil, err
				}
				session = res.Msg.GetUpload().GetUploadId()
				size := min(int(res.Msg.GetUpload().GetMaxChunkSize()), 1<<20)
				if session == "" || size <= 0 {
					f.Close()
					return nil, errors.New("import: invalid upload session")
				}
				err = func() error {
					defer f.Close()
					chunk := make([]byte, size)
					for offset := int64(0); offset < a.Storage.Size; {
						count := min(int64(size), a.Storage.Size-offset)
						if _, err := io.ReadFull(f, chunk[:count]); err != nil {
							return err
						}
						h := sha256.Sum256(chunk[:count])
						if _, err := m.assetAPI.UploadChunk(ctx, operatorRequest(&op.UploadChunkRequest{UploadId: session, Offset: offset, Content: chunk[:count], ChunkSha256: hex.EncodeToString(h[:])})); err != nil {
							return err
						}
						offset += count
					}
					var extra [1]byte
					if n, err := f.Read(extra[:]); n != 0 || err != io.EOF {
						return errors.New("import: asset size changed since validation")
					}
					return nil
				}()
				if err != nil {
					return nil, err
				}
				// CompleteUpload is exactly retryable. Persist its session before
				// sending it; a lost acknowledgement must not start another upload.
				if err = m.progress.put(key+"/complete", session); err != nil {
					return nil, err
				}
			}
			res, err := m.assetAPI.CompleteUpload(ctx, operatorRequest(&op.CompleteUploadRequest{UploadId: session}))
			if err != nil {
				return nil, err
			}
			native = res.Msg.AssetId
			if err = m.progress.put(key, native); err != nil {
				return nil, err
			}
		}
		result = append(result, native)
	}
	return result, nil
}
