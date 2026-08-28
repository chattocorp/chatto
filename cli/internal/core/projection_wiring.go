package core

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/log"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/notificationstream"
	"hmans.de/chatto/internal/projectionsnapshot"
	"hmans.de/chatto/pkg/events"
)

// coreProjections is the complete construction result for core-owned
// projections. Its registration slice is the single source used by runtime
// lifecycle, readiness, and operator diagnostics.
type coreProjections struct {
	registrations []projectionRegistration
	snapshotJobs  []projectionSnapshotJob

	roomDirectory         events.ProjectionHandle[*RoomDirectoryProjection]
	notificationDecisions events.ProjectionHandle[*NotificationDecisionProjection]
	notifications         events.ProjectionHandle[*NotificationProjection]
	serverConfig          events.ProjectionHandle[*ConfigProjection]
	roomGroupLayout       events.ProjectionHandle[*RoomGroupLayoutProjection]
	roomTimeline          events.ProjectionHandle[*RoomTimelineProjection]
	callState             events.ProjectionHandle[*CallStateProjection]
	assets                events.ProjectionHandle[*AssetProjection]
	threads               events.ProjectionHandle[*ThreadProjection]
	reactions             events.ProjectionHandle[*ReactionProjection]
	users                 events.ProjectionHandle[*UserProjection]
	userAuth              events.ProjectionHandle[*UserAuthProjection]
	contentKeys           events.ProjectionHandle[*ContentKeyProjection]
	rbac                  events.ProjectionHandle[*RBACProjection]
	mentionables          events.ProjectionHandle[*MentionablesProjection]
	invitations           events.ProjectionHandle[*InvitationProjection]
	oauthClients          events.ProjectionHandle[*OAuthClientProjection]
}

type projectionSnapshotPolicy bool

const (
	coldReplayOnly  projectionSnapshotPolicy = false
	sharedSnapshots projectionSnapshotPolicy = true
)

// projectionRegistrar keeps projector construction and diagnostic
// registration atomic so those inventories cannot drift apart.
type projectionRegistrar struct {
	infra         *coreInfrastructure
	logger        *log.Logger
	registrations []projectionRegistration
}

func registerProjectionHandle[P events.SubjectProjection](
	r *projectionRegistrar,
	handle events.ProjectionHandle[P],
	projection P,
	key string,
	name string,
	streamName string,
	identityResolver events.StreamIdentityResolver,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
) events.ProjectionHandle[P] {
	r.registrations = append(r.registrations, projectionRegistration{
		key:              key,
		name:             name,
		projector:        handle.Projector(),
		subjects:         slices.Clone(projection.Subjects()),
		snapshotPolicy:   snapshotPolicy,
		streamName:       streamName,
		identityResolver: identityResolver,
		estimate:         estimate,
	})
	return handle
}

func registerProjection[T any, P evtstream.ProjectionPointer[T]](
	r *projectionRegistrar,
	projection P,
	key string,
	name string,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
) events.ProjectionHandle[P] {
	loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
	handle := evtstream.NewProjectionHandle(
		r.infra.js,
		r.infra.storage.serverEvtStream,
		projection,
		r.logger.WithPrefix("core."+loggerName),
	)
	return registerProjectionHandle(
		r,
		handle,
		projection,
		key,
		name,
		r.infra.storage.serverEvtStream.CachedInfo().Config.Name,
		evtstream.IdentityFromInfo,
		estimate,
		snapshotPolicy,
	)
}

func initializeCoreProjections(
	infra *coreInfrastructure,
	logger *log.Logger,
) (*coreProjections, error) {
	registrar := &projectionRegistrar{infra: infra, logger: logger}
	projections := &coreProjections{}

	roomDirectory := NewRoomDirectoryProjection()
	projections.roomDirectory = registerProjection(
		registrar,
		roomDirectory,
		projectionsnapshot.ProjectionRoomDirectoryKey,
		"Room Directory",
		roomDirectory.adminProjectionEstimate,
		sharedSnapshots,
	)

	notificationDecisions := NewNotificationDecisionProjection()
	projections.notificationDecisions = registerProjection(
		registrar,
		notificationDecisions,
		projectionsnapshot.ProjectionNotificationDecisionsKey,
		"Notification Decisions",
		notificationDecisions.adminProjectionEstimate,
		sharedSnapshots,
	)

	notifications := NewNotificationProjection()
	notificationHandle := notificationstream.NewProjectionHandle(
		infra.js,
		infra.storage.notificationStream,
		notifications,
		logger.WithPrefix("core.NotificationsProjector"),
	)
	projections.notifications = registerProjectionHandle(
		registrar,
		notificationHandle,
		notifications,
		projectionsnapshot.ProjectionNotificationsKey,
		"Notifications",
		infra.storage.notificationStream.CachedInfo().Config.Name,
		notificationstream.IdentityFromInfo,
		notifications.adminProjectionEstimate,
		sharedSnapshots,
	)

	serverConfig := NewConfigProjection()
	projections.serverConfig = registerProjection(
		registrar,
		serverConfig,
		projectionsnapshot.ProjectionServerConfigKey,
		"Server Config",
		serverConfig.adminProjectionEstimate,
		sharedSnapshots,
	)

	roomGroupLayout := NewRoomGroupLayoutProjection()
	projections.roomGroupLayout = registerProjection(
		registrar,
		roomGroupLayout,
		projectionsnapshot.ProjectionRoomGroupLayoutKey,
		"Room Group Layout",
		roomGroupLayout.adminProjectionEstimate,
		sharedSnapshots,
	)

	roomTimeline := NewRoomTimelineProjection()
	projections.roomTimeline = registerProjection(
		registrar,
		roomTimeline,
		projectionsnapshot.ProjectionRoomTimelineKey,
		"Room Timeline",
		roomTimeline.adminProjectionEstimate,
		sharedSnapshots,
	)

	callState := NewCallStateProjection()
	projections.callState = registerProjection(
		registrar,
		callState,
		projectionsnapshot.ProjectionCallStateKey,
		"Call State",
		callState.adminProjectionEstimate,
		sharedSnapshots,
	)

	assets := NewAssetProjection()
	projections.assets = registerProjection(
		registrar,
		assets,
		projectionsnapshot.ProjectionAssetsKey,
		"Assets",
		assets.adminProjectionEstimate,
		sharedSnapshots,
	)

	threads := NewThreadProjection()
	projections.threads = registerProjection(
		registrar,
		threads,
		projectionsnapshot.ProjectionThreadsKey,
		"Threads",
		threads.adminProjectionEstimate,
		sharedSnapshots,
	)

	reactions := NewReactionProjection()
	projections.reactions = registerProjection(
		registrar,
		reactions,
		projectionsnapshot.ProjectionReactionsKey,
		"Reactions",
		reactions.adminProjectionEstimate,
		sharedSnapshots,
	)

	users := newUserProjectionWithDEKResolver(infra.dekResolver)
	projections.users = registerProjection(
		registrar,
		users,
		projectionsnapshot.ProjectionUsersKey,
		"Users",
		users.adminProjectionEstimate,
		sharedSnapshots,
	)
	userAuth := users.AuthProjection()
	projections.userAuth = registerProjection(
		registrar,
		userAuth,
		"user_auth",
		"User Auth",
		userAuth.adminProjectionEstimate,
		coldReplayOnly,
	)

	contentKeys := NewContentKeyProjection()
	projections.contentKeys = registerProjection(
		registrar,
		contentKeys,
		projectionsnapshot.ProjectionContentKeysKey,
		"Content Keys",
		contentKeys.adminProjectionEstimate,
		sharedSnapshots,
	)

	rbac := NewRBACProjection()
	projections.rbac = registerProjection(
		registrar,
		rbac,
		projectionsnapshot.ProjectionRBACKey,
		"RBAC",
		rbac.adminProjectionEstimate,
		sharedSnapshots,
	)

	mentionables := newMentionablesProjectionWithDEKResolver(infra.dekResolver)
	projections.mentionables = registerProjection(
		registrar,
		mentionables,
		projectionsnapshot.ProjectionMentionablesKey,
		"Mentionables",
		mentionables.adminProjectionEstimate,
		sharedSnapshots,
	)

	invitations := NewInvitationProjection()
	projections.invitations = registerProjection(
		registrar,
		invitations,
		"invitations",
		"Invitations",
		invitations.adminProjectionEstimate,
		coldReplayOnly,
	)

	oauthClients := NewOAuthClientProjection()
	projections.oauthClients = registerProjection(
		registrar,
		oauthClients,
		"oauth_clients",
		"OAuth Clients",
		oauthClients.adminProjectionEstimate,
		coldReplayOnly,
	)

	projections.registrations = registrar.registrations
	if err := configureProjectionSnapshots(infra, projections); err != nil {
		return nil, err
	}
	return projections, nil
}

func configureProjectionSnapshots(
	infra *coreInfrastructure,
	projections *coreProjections,
) error {
	if infra.snapshotRepository == nil {
		return nil
	}

	for i := range projections.registrations {
		registration := &projections.registrations[i]
		if registration.snapshotPolicy == coldReplayOnly {
			continue
		}
		source := events.ProjectionSnapshotSource(projectionSnapshotSource{repository: infra.snapshotRepository})
		if registration.key == projectionsnapshot.ProjectionNotificationDecisionsKey {
			source = cappedNotificationDecisionSnapshotSource{
				source:     source,
				projection: projections.notificationDecisions.Projection(),
			}
		}
		if err := registration.projector.ConfigureSnapshots(
			registration.key,
			source,
			registration.identityResolver,
		); err != nil {
			return fmt.Errorf("configure %s projection snapshots: %w", registration.key, err)
		}
		job := projectionSnapshotJob{
			projector:     registration.projector,
			repository:    infra.snapshotRepository,
			projectionKey: registration.key,
			streamName:    registration.streamName,
		}
		if registration.key == projectionsnapshot.ProjectionNotificationDecisionsKey {
			job.allowPublication = projections.notificationDecisions.Projection().AllowSnapshotPublication
		}
		projections.snapshotJobs = append(projections.snapshotJobs, job)
		registration.snapshotEnabled = true
	}
	return nil
}
