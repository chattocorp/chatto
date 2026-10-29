//go:build bootstrap || test_endpoints

package cmd

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/assets"
	"github.com/chattocorp/chatto/cli/internal/core"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

type seedAvatarTransport func(*http.Request) (*http.Response, error)

func (f seedAvatarTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestDownloadSeedAvatarsBoundsPoolAndUsesSyntheticKeys(t *testing.T) {
	var portrait bytes.Buffer
	require.NoError(t, png.Encode(&portrait, image.NewRGBA(image.Rect(0, 0, 16, 16))))
	var keys []string
	client := &http.Client{Transport: seedAvatarTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "https", req.URL.Scheme)
		require.Equal(t, "i.pravatar.cc", req.URL.Host)
		require.Equal(t, "/128", req.URL.Path)
		keys = append(keys, req.URL.Query().Get("u"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(portrait.Bytes())), Header: make(http.Header)}, nil
	})}
	images, err := downloadSeedAvatars(context.Background(), client, -42, 5000)
	require.NoError(t, err)
	require.Len(t, images, 20)
	require.Equal(t, "chatto-seed--42-0", keys[0])
	require.Equal(t, "chatto-seed--42-19", keys[19])
	_, err = assets.InspectImage(bytes.NewReader(images[0]))
	require.NoError(t, err)
}

func TestDownloadSeedAvatarsRejectsFailuresWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate limit", 429, "slow down"},
		{"HTML", 200, "<html>not a portrait</html>"},
		{"oversized", 200, strings.Repeat("x", (1<<20)+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: seedAvatarTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header)}, nil
			})}
			images, err := downloadSeedAvatars(context.Background(), client, 42, 2)
			require.Error(t, err)
			require.Nil(t, images)
			require.Equal(t, 1, calls)
		})
	}
}

func TestSeedAvatarCancellationAndRedirects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &http.Client{Transport: seedAvatarTransport(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})}
	_, err := downloadSeedAvatars(ctx, client, 42, 1)
	require.ErrorIs(t, err, context.Canceled)
	for _, address := range []string{"http://i.pravatar.cc/128", "https://example.com/image", "https://localhost/image"} {
		parsed, err := url.Parse(address)
		require.NoError(t, err)
		require.Error(t, seedAvatarRedirect(&http.Request{URL: parsed}, nil))
	}
	parsed, err := url.Parse("https://i.pravatar.cc/128?img=1")
	require.NoError(t, err)
	require.NoError(t, seedAvatarRedirect(&http.Request{URL: parsed}, nil))
	require.Error(t, seedAvatarRedirect(&http.Request{URL: parsed}, make([]*http.Request, 3)))
}

func TestSeedAvatarSourcesAreMutuallyExclusive(t *testing.T) {
	cmd := operatorSeedCmd()
	cmd.SetArgs([]string{"--pravatar", "--avatar-dir", "unused"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	require.ErrorContains(t, cmd.Execute(), "none of the others can be")
}

func TestSeedCLIUploadsLocalAvatarsWithNotificationHistory(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "avatar-seed-viewer", "Viewer", "")
	require.NoError(t, err)
	directory := t.TempDir()
	var portrait bytes.Buffer
	require.NoError(t, png.Encode(&portrait, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "portrait.png"), portrait.Bytes(), 0600))
	output := env.run(t, "operator", "seed", "--users", "2", "--rooms", "1", "--messages", "0", "--avatar-dir", directory,
		"--notification-recipient", viewer.Login, "--notifications", "6", "--notification-days", "2", "--json")
	var result operatorv1.SeedDataResponse
	require.NoError(t, protojson.Unmarshal([]byte(output), &result))
	require.EqualValues(t, 6, result.NotificationCount)
	occurrences, err := env.core.NotificationOccurrences().List(env.ctx, viewer.Id)
	require.NoError(t, err)
	require.Len(t, occurrences, 6)
	for _, seeded := range result.Users {
		avatarURL, err := env.core.GetUserAvatarURL(env.ctx, seeded.Id, nil, nil, "")
		require.NoError(t, err)
		require.NotEmpty(t, avatarURL)
	}
}

func TestReadSeedAvatarImages(t *testing.T) {
	directory := t.TempDir()
	_, err := readSeedAvatarImages(directory)
	require.ErrorContains(t, err, "contains no")
	var portrait bytes.Buffer
	require.NoError(t, png.Encode(&portrait, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "b.png"), portrait.Bytes(), 0600))
	portrait.Reset()
	require.NoError(t, png.Encode(&portrait, image.NewRGBA(image.Rect(0, 0, 16, 16))))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "a.png"), portrait.Bytes(), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "notes.txt"), []byte("ignored"), 0600))
	images, err := readSeedAvatarImages(directory)
	require.NoError(t, err)
	require.Len(t, images, 2)
	first, err := assets.InspectImage(bytes.NewReader(images[0]))
	require.NoError(t, err)
	second, err := assets.InspectImage(bytes.NewReader(images[1]))
	require.NoError(t, err)
	require.Equal(t, 16, first.Width)
	require.Equal(t, 32, second.Width)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "invalid.png"), []byte("invalid image"), 0600))
	_, err = readSeedAvatarImages(directory)
	require.ErrorContains(t, err, "read seed avatar")
}

func TestSeedCLIUsesOperatorSocket(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	output := env.run(t, "operator", "seed", "--seed", "73", "--users", "2", "--rooms", "1", "--messages", "4", "--thread-replies", "2", "--json")
	var result operatorv1.SeedDataResponse
	require.NoError(t, protojson.Unmarshal([]byte(output), &result))
	require.EqualValues(t, 73, result.Seed)
	require.Len(t, result.Users, 2)
	require.Len(t, result.Rooms, 1)
	require.Len(t, result.Messages, 4)
	require.NotEmpty(t, result.Rooms[0].MemberIds)
	for _, userID := range result.Rooms[0].MemberIds {
		member, err := env.core.RoomMemberships().GetRoomMembership(env.ctx, core.KindChannel, userID, result.Rooms[0].Id)
		require.NoError(t, err)
		require.Equal(t, userID, member.UserId)
	}
	for _, message := range result.Messages {
		body, err := env.core.GetMessageBody(env.ctx, message.Id)
		require.NoError(t, err)
		require.Equal(t, message.Body, body)
	}
}

func TestSeedCLIExplainsMissingOrStoppedServer(t *testing.T) {
	resetOperatorGlobals(t)
	// Keep Unix socket names below the macOS path limit.
	directory, err := os.MkdirTemp("/tmp", "seed-cli-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(directory)) })
	operatorSocketPath = filepath.Join(directory, "operator.sock")
	for _, stopped := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "refused"}[stopped], func(t *testing.T) {
			if stopped {
				listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: operatorSocketPath, Net: "unix"})
				require.NoError(t, err)
				listener.SetUnlinkOnClose(false)
				require.NoError(t, listener.Close())
			}
			cmd := operatorSeedCmd()
			cmd.SetContext(context.Background())
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"--users", "1", "--rooms", "1", "--messages", "0"})
			err := cmd.Execute()
			require.ErrorContains(t, err, "start or restart mise dev")
			require.ErrorContains(t, err, "no data was created")
			require.NotContains(t, err.Error(), "partial data")
		})
	}
}

func TestSeedCLINotificationHistory(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "seed-viewer", "Viewer", "")
	require.NoError(t, err)
	output := env.run(t, "operator", "seed", "--users", "2", "--rooms", "1", "--messages", "0",
		"--notification-recipient", viewer.Login, "--notifications", "6", "--notification-days", "2", "--json")
	var result operatorv1.SeedDataResponse
	require.NoError(t, protojson.Unmarshal([]byte(output), &result))
	require.EqualValues(t, 6, result.NotificationCount)
	occurrences, err := env.core.NotificationOccurrences().List(env.ctx, viewer.Id)
	require.NoError(t, err)
	require.Len(t, occurrences, 6)
}

func TestSeedCLIKeepsPartialDataWarningAfterConnection(t *testing.T) {
	for _, err := range []error{
		connect.NewError(connect.CodeUnavailable, io.EOF),
		&net.OpError{Op: "write", Net: "unix", Err: syscall.EPIPE},
		context.DeadlineExceeded,
	} {
		result := seedCommandError(err)
		require.ErrorContains(t, result, "partial data may remain")
		require.NotContains(t, result.Error(), "no data was created")
		require.ErrorIs(t, result, err)
	}
}
