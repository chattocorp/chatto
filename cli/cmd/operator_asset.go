package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

var operatorAssetCmd = &cobra.Command{
	Use:   "asset",
	Short: "Manage local attachment assets",
}

func init() {
	operatorCmd.AddCommand(operatorAssetCmd)
	operatorAssetCmd.AddCommand(operatorAssetUploadCmd())
}

func operatorAssetUploadCmd() *cobra.Command {
	var roomID, authorID, path, filename, contentType string
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a local attachment for a channel room author",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(roomID) == "" || strings.TrimSpace(authorID) == "" || strings.TrimSpace(path) == "" {
				return errors.New("--room-id, --author-id, and --file are required")
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return errors.New("--file must be a regular file")
			}
			if filename == "" {
				filename = filepath.Base(path)
			}
			if contentType == "" {
				contentType = mime.TypeByExtension(filepath.Ext(filename))
				if contentType == "" {
					contentType = "application/octet-stream"
				}
			}
			hash := sha256.New()
			if _, err := io.Copy(hash, file); err != nil {
				return err
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			client, err := newOperatorAssetClient()
			if err != nil {
				return err
			}
			created, err := client.CreateUpload(cmd.Context(), operatorRequest(&operatorv1.CreateUploadRequest{
				RoomId: roomID, AuthorId: authorID, Filename: filename, ContentType: contentType,
				Size: info.Size(), Sha256: hex.EncodeToString(hash.Sum(nil)),
			}))
			if err != nil {
				return err
			}
			uploadID := created.Msg.GetUpload().GetUploadId()
			maxChunkSize := created.Msg.GetUpload().GetMaxChunkSize()
			completed := false
			defer func() {
				if completed || uploadID == "" {
					return
				}
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, _ = client.CancelUpload(cleanupCtx, operatorRequest(&operatorv1.CancelUploadRequest{UploadId: uploadID}))
			}()
			if uploadID == "" || maxChunkSize <= 0 {
				return errors.New("server returned an invalid upload session")
			}
			chunkSize := min(int(maxChunkSize), 1024*1024)
			chunk := make([]byte, chunkSize)
			for offset := int64(0); offset < info.Size(); {
				count := min(int64(chunkSize), info.Size()-offset)
				if _, err := io.ReadFull(file, chunk[:count]); err != nil {
					return fmt.Errorf("read upload file: %w", err)
				}
				sum := sha256.Sum256(chunk[:count])
				if _, err := client.UploadChunk(cmd.Context(), operatorRequest(&operatorv1.UploadChunkRequest{
					UploadId: uploadID, Offset: offset, Content: chunk[:count], ChunkSha256: hex.EncodeToString(sum[:]),
				})); err != nil {
					return err
				}
				offset += count
			}
			result, err := client.CompleteUpload(cmd.Context(), operatorRequest(&operatorv1.CompleteUploadRequest{UploadId: uploadID}))
			if err != nil {
				return err
			}
			completed = true
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, result.Msg, func() { fmt.Fprintln(out, result.Msg.GetAssetId()) })
		},
	}
	cmd.Flags().StringVar(&roomID, "room-id", "", "channel room ID")
	cmd.Flags().StringVar(&authorID, "author-id", "", "mapped Chatto author ID")
	cmd.Flags().StringVar(&path, "file", "", "local attachment file")
	cmd.Flags().StringVar(&filename, "filename", "", "stored filename (default: file basename)")
	cmd.Flags().StringVar(&contentType, "content-type", "", "MIME type (default: inferred from filename)")
	return cmd
}

func newOperatorAssetClient() (operatorv1connect.OperatorAssetServiceClient, error) {
	httpClient, baseURL, err := newOperatorHTTPClient()
	if err != nil {
		return nil, err
	}
	return operatorv1connect.NewOperatorAssetServiceClient(httpClient, baseURL), nil
}
