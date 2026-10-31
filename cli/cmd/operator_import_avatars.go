package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/chattocorp/chatto/cli/internal/core"
	op "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
)

// importAvatars fills empty destination avatars. Shared email identities use
// the first supplied avatar; the API preserves existing and concurrently set ones.
func (m *cliArchiveImport) importAvatars(ctx context.Context) error {
	for i, u := range m.archive.Users {
		if u.AvatarAssetID == "" {
			continue
		}
		key := "avatar/" + importRef(u.ID)
		if m.progress.values[key] != "" {
			continue
		}
		asset := m.assets[u.AvatarAssetID]
		f, err := m.archive.OpenAsset(asset)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(f, core.MaxHistoricalAvatarBytes+1))
		f.Close()
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if int64(len(data)) != asset.Storage.Size || hex.EncodeToString(sum[:]) != asset.Storage.SHA256 {
			return fmt.Errorf("user avatar bytes changed after validation")
		}
		_, err = m.userAPI.ImportUserAvatar(ctx, operatorRequest(&op.ImportUserAvatarRequest{UserId: m.users[u.ID], OperationKey: m.progress.operation(key), Image: data}))
		if err != nil {
			return fmt.Errorf("import avatar for user record %d: %w", i+1, err)
		}
		if err := m.progress.put(key, "done"); err != nil {
			return err
		}
	}
	return nil
}
