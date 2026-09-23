package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
	"hmans.de/chatto/internal/pb/chatto/operator/v1/operatorv1connect"
)

var operatorMessageCmd = &cobra.Command{Use: "message", Short: "Manage historical messages"}

func init() {
	operatorCmd.AddCommand(operatorMessageCmd)
	operatorMessageCmd.AddCommand(operatorMessageImportCmd())
}

func operatorMessageImportCmd() *cobra.Command {
	var roomID, authorID, createdAt, editedAt, inReplyTo, body, bodyFile string
	var previewURL, previewTitle, previewDescription, previewType string
	var bodyStdin bool
	var assetIDs []string
	cmd := &cobra.Command{
		Use: "import", Short: "Import one historical channel message", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(roomID) == "" || strings.TrimSpace(authorID) == "" || createdAt == "" {
				return errors.New("--room-id, --author-id, and --created-at are required")
			}
			bodySources := 0
			for _, selected := range []bool{cmd.Flags().Changed("body"), cmd.Flags().Changed("body-file"), bodyStdin} {
				if selected {
					bodySources++
				}
			}
			if bodySources > 1 {
				return errors.New("use only one of --body, --body-file, or --body-stdin")
			}
			if cmd.Flags().Changed("body-file") && strings.TrimSpace(bodyFile) == "" {
				return errors.New("--body-file needs a path")
			}
			if bodyFile != "" {
				file, err := os.Open(bodyFile)
				if err != nil {
					return err
				}
				defer file.Close()
				info, err := file.Stat()
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return errors.New("--body-file must be a regular file")
				}
				bodyBytes, err := io.ReadAll(io.LimitReader(file, int64(core.MaxMessageBodyLength)+1))
				if err != nil {
					return err
				}
				body = string(bodyBytes)
			}
			if bodyStdin {
				bodyBytes, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), int64(core.MaxMessageBodyLength)+1))
				if err != nil {
					return err
				}
				body = string(bodyBytes)
			}
			if len(body) > core.MaxMessageBodyLength {
				return fmt.Errorf("message body exceeds %d bytes", core.MaxMessageBodyLength)
			}
			created, err := time.Parse(time.RFC3339Nano, createdAt)
			if err != nil {
				return fmt.Errorf("--created-at: %w", err)
			}
			request := &operatorv1.ImportMessageRequest{
				RoomId: roomID, AuthorId: authorID, CreatedAt: timestamppb.New(created),
				InReplyTo: inReplyTo, AttachmentAssetIds: assetIDs, Body: body,
				PreviewUrl: previewURL, PreviewTitle: previewTitle, PreviewDescription: previewDescription, PreviewType: previewType,
			}
			if editedAt != "" {
				edited, err := time.Parse(time.RFC3339Nano, editedAt)
				if err != nil {
					return fmt.Errorf("--edited-at: %w", err)
				}
				request.EditedAt = timestamppb.New(edited)
			}
			client, err := newOperatorMessageClient()
			if err != nil {
				return err
			}
			response, err := client.ImportMessage(cmd.Context(), operatorRequest(request))
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			return printOperatorOutput(out, response.Msg, func() { fmt.Fprintln(out, response.Msg.GetMessageId()) })
		},
	}
	cmd.Flags().StringVar(&roomID, "room-id", "", "channel room ID")
	cmd.Flags().StringVar(&authorID, "author-id", "", "mapped Chatto author ID")
	cmd.Flags().StringVar(&createdAt, "created-at", "", "original creation time in RFC3339")
	cmd.Flags().StringVar(&editedAt, "edited-at", "", "original edit time in RFC3339")
	cmd.Flags().StringVar(&inReplyTo, "in-reply-to", "", "Chatto message ID in the same room")
	cmd.Flags().StringSliceVar(&assetIDs, "attachment-asset-id", nil, "attachment asset ID (repeatable)")
	cmd.Flags().StringVar(&body, "body", "", "message body text")
	cmd.Flags().StringVar(&bodyFile, "body-file", "", "read message body from a local file")
	cmd.Flags().BoolVar(&bodyStdin, "body-stdin", false, "read message body from standard input")
	cmd.Flags().StringVar(&previewURL, "preview-url", "", "exported preview URL")
	cmd.Flags().StringVar(&previewTitle, "preview-title", "", "exported preview title")
	cmd.Flags().StringVar(&previewDescription, "preview-description", "", "exported preview description")
	cmd.Flags().StringVar(&previewType, "preview-type", "", "exported preview type")
	return cmd
}

func newOperatorMessageClient() (operatorv1connect.OperatorMessageServiceClient, error) {
	httpClient, baseURL, err := newOperatorHTTPClient()
	if err != nil {
		return nil, err
	}
	return operatorv1connect.NewOperatorMessageServiceClient(httpClient, baseURL), nil
}
