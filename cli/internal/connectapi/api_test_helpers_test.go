package connectapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/nats-io/nats.go"

	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func requireConnectCode(t testing.TB, err error, want connect.Code) {
	t.Helper()
	if got := connect.CodeOf(err); got != want {
		t.Fatalf("connect code = %v, want %v (err = %v)", got, want, err)
	}
}

func withCaller(ctx context.Context, user *corev1.User) context.Context {
	return authn.SetInfo(ctx, Caller{UserID: user.Id})
}

func withBearerCredential(ctx context.Context, user *corev1.User, token string) context.Context {
	ctx = withCaller(ctx, user)
	return authctx.WithCredential(ctx, authctx.RuntimeCredential{
		Kind:   authctx.RuntimeCredentialKindBearerToken,
		UserID: user.Id,
		Handle: token,
	})
}

func boolPtr(value bool) *bool {
	return &value
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func findAPIPermissionCell(cells []*adminv1.PermissionMatrixCell, scopeID, permission string) *adminv1.PermissionMatrixCell {
	for _, cell := range cells {
		if cell.GetScopeId() == scopeID && cell.GetPermission() == permission {
			return cell
		}
	}
	return nil
}

func findAPIPermissionDecision(decisions []*adminv1.ScopedPermissionDecision, kind adminv1.PermissionScopeKind, scopeID, permission string) *adminv1.ScopedPermissionDecision {
	for _, decision := range decisions {
		scope := decision.GetScope()
		if scope != nil && scope.GetKind() == kind && scope.GetId() == scopeID && decision.GetPermission() == permission {
			return decision
		}
	}
	return nil
}

func findAPITierRole(roles []*adminv1.TierRole, roleName string) *adminv1.TierRole {
	for _, role := range roles {
		if role.GetRole().GetName() == roleName {
			return role
		}
	}
	return nil
}

func connectAPITestPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 180, G: 60, B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

type connectAPITestEnv struct {
	ctx              context.Context
	core             *core.ChattoCore
	nc               *nats.Conn
	api              *API
	account          *accountService
	adminDiagnostics *adminDiagnosticsService
	adminEventLog    *adminEventLogService
	adminLayout      *adminRoomLayoutService
	adminUsers       *adminUserManagementService
	assets           *assetService
	assetUploads     *assetUploadService
	directory        *roomDirectoryService
	externalAuth     *externalIdentityAuthService
	messages         *messageService
	notifications    *notificationService
	permissions      *permissionService
	prefs            *notificationPreferencesService
	push             *pushNotificationService
	publicRoles      *publicRoleService
	roles            *roleService
	rooms            *roomService
	serverState      *serverService
	threads          *threadService
	users            *userService
	viewerService    *viewerService
	voice            *voiceCallService
	viewer           *corev1.User
}

func newConnectAPITestEnv(t *testing.T) *connectAPITestEnv {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	c, err := core.NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startConnectAPITestCore(t, c)

	viewer, err := c.CreateUser(ctx, core.SystemActorID, "timeline-viewer", "Timeline Viewer", "password")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	api := New(c, config.ChattoConfig{}, "test")
	return &connectAPITestEnv{
		ctx:              ctx,
		core:             c,
		nc:               nc,
		api:              api,
		account:          &accountService{api: api},
		adminDiagnostics: &adminDiagnosticsService{api: api},
		adminEventLog:    &adminEventLogService{api: api},
		adminLayout:      &adminRoomLayoutService{api: api},
		adminUsers:       &adminUserManagementService{api: api},
		assets:           &assetService{api: api},
		assetUploads:     &assetUploadService{api: api},
		directory:        &roomDirectoryService{api: api},
		externalAuth:     &externalIdentityAuthService{api: api},
		messages:         &messageService{api: api},
		notifications:    &notificationService{api: api},
		permissions:      &permissionService{api: api},
		prefs:            &notificationPreferencesService{api: api},
		push:             &pushNotificationService{api: api},
		publicRoles:      &publicRoleService{api: api},
		roles:            &roleService{api: api},
		rooms:            &roomService{api: api},
		serverState:      &serverService{api: api},
		threads:          &threadService{api: api},
		users:            &userService{api: api},
		viewerService:    &viewerService{api: api},
		voice:            &voiceCallService{api: api},
		viewer:           viewer,
	}
}

func startConnectAPITestCore(t *testing.T, c *core.ChattoCore) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := c.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

func apiCapabilityGranted(grants []*apiv1.CapabilityGrant, capability string) bool {
	for _, grant := range grants {
		if grant.GetCapability() == capability {
			return grant.GetGranted()
		}
	}
	return false
}

func apiPermissionGrantPresent(grants []*apiv1.PermissionGrant, permission string) bool {
	for _, grant := range grants {
		if grant.GetPermission() == permission {
			return true
		}
	}
	return false
}

func apiRoomPermissionGranted(room *apiv1.RoomWithViewerState, permission core.Permission) bool {
	return apiPermissionGranted(room.GetViewerState().GetPermissions(), string(permission))
}

func apiRoomGroupPermissionGranted(group *apiv1.RoomGroup, permission core.Permission) bool {
	return apiPermissionGranted(group.GetViewerState().GetPermissions(), string(permission))
}

func apiPermissionGranted(grants []*apiv1.PermissionGrant, permission string) bool {
	for _, grant := range grants {
		if grant.GetPermission() == permission {
			return grant.GetGranted()
		}
	}
	return false
}

func (e *connectAPITestEnv) createJoinedRoom(name string) *corev1.Room {
	room, err := e.core.CreateRoom(e.ctx, e.viewer.Id, core.KindChannel, "", name, "")
	if err != nil {
		panic(err)
	}
	if _, err := e.core.JoinRoom(e.ctx, e.viewer.Id, core.KindChannel, e.viewer.Id, room.Id); err != nil {
		panic(err)
	}
	return room
}

func (e *connectAPITestEnv) uploadAttachmentAsset(t testing.TB, roomID, filename, contentType string, content []byte) string {
	t.Helper()
	sum := sha256.Sum256(content)
	ctx := withCaller(e.ctx, e.viewer)
	created, err := e.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
		RoomId:      roomID,
		Filename:    filename,
		ContentType: contentType,
		Size:        int64(len(content)),
		Sha256:      hex.EncodeToString(sum[:]),
	}))
	if err != nil {
		t.Fatalf("CreateUpload: %v", err)
	}
	chunkSum := sha256.Sum256(content)
	if _, err := e.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
		UploadId:    created.Msg.GetUpload().GetUploadId(),
		Content:     content,
		ChunkSha256: hex.EncodeToString(chunkSum[:]),
	})); err != nil {
		t.Fatalf("UploadChunk: %v", err)
	}
	completed, err := e.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
		UploadId: created.Msg.GetUpload().GetUploadId(),
	}))
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	assetID := completed.Msg.GetAsset().GetId()
	if assetID == "" {
		t.Fatal("completed upload asset id is empty")
	}
	return assetID
}

func (e *connectAPITestEnv) defaultRoomGroupID(t *testing.T) string {
	t.Helper()
	groups, err := e.core.ListRoomGroupsOrdered(e.ctx, core.KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatalf("expected at least one default room group")
	}
	return groups[0].Id
}

func (e *connectAPITestEnv) post(roomID, actorID, body, inReplyTo string) *corev1.Event {
	event, err := e.core.PostMessage(e.ctx, core.KindChannel, roomID, actorID, body, nil, inReplyTo, "", nil, false)
	if err != nil {
		panic(err)
	}
	return event
}

func timelinePageContains(page *apiv1.RoomTimelinePage, eventID string) bool {
	return timelinePageEvent(page, eventID) != nil
}

func timelinePageEvent(page *apiv1.RoomTimelinePage, eventID string) *apiv1.RoomTimelineEvent {
	for _, event := range page.Events {
		if event.Id == eventID {
			return event
		}
	}
	return nil
}

func timelinePageEventIDs(page *apiv1.RoomTimelinePage) []string {
	ids := make([]string, 0, len(page.Events))
	for _, event := range page.Events {
		ids = append(ids, event.Id)
	}
	return ids
}

func directoryRoomsByID(rooms []*apiv1.RoomWithViewerState) map[string]*apiv1.RoomWithViewerState {
	result := make(map[string]*apiv1.RoomWithViewerState, len(rooms))
	for _, room := range rooms {
		if room == nil || room.GetRoom() == nil {
			continue
		}
		result[room.GetRoom().GetId()] = room
	}
	return result
}

func findDirectoryGroup(groups []*apiv1.RoomGroup, groupID string) *apiv1.RoomGroup {
	for _, group := range groups {
		if group.GetId() == groupID {
			return group
		}
	}
	return nil
}

func findAdminRoomLayoutGroup(groups []*adminv1.AdminRoomLayoutGroup, groupID string) *adminv1.AdminRoomLayoutGroup {
	for _, group := range groups {
		if group.GetId() == groupID {
			return group
		}
	}
	return nil
}

func roomGroupItemsContainRoom(items []*apiv1.RoomGroupItem, roomID string) bool {
	for _, item := range items {
		room := item.GetRoom()
		if room != nil && room.GetRoom().GetId() == roomID {
			return true
		}
	}
	return false
}

func adminRoomLayoutItemsContainRoom(items []*adminv1.AdminRoomLayoutItem, roomID string) bool {
	for _, item := range items {
		room := item.GetRoom()
		if room != nil && room.GetId() == roomID {
			return true
		}
	}
	return false
}

func roomGroupItemsContainSidebarLink(items []*apiv1.RoomGroupItem, linkID string) bool {
	for _, item := range items {
		link := item.GetSidebarLink()
		if link != nil && link.GetId() == linkID {
			return true
		}
	}
	return false
}
