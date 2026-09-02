package core

import (
	"context"
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
	contentView   *ServerContentView

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
	ctx           context.Context
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
) (events.ProjectionHandle[P], error) {
	loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
	streamName := r.infra.storage.serverEvtStream.CachedInfo().Config.Name
	stream, err := r.infra.js.Stream(r.ctx, streamName)
	if err != nil {
		return events.ProjectionHandle[P]{}, fmt.Errorf("open EVT stream for %s: %w", name, err)
	}
	handle := evtstream.NewProjectionHandle(
		r.infra.js,
		stream,
		projection,
		r.logger.WithPrefix("core."+loggerName),
	)
	return registerProjectionHandle(
		r,
		handle,
		projection,
		key,
		name,
		streamName,
		evtstream.IdentityFromInfo,
		estimate,
		snapshotPolicy,
	), nil
}

func registerPreparedProjection[T any, P evtstream.PreparedProjectionPointer[T]](
	r *projectionRegistrar,
	projection P,
	key string,
	name string,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
) (events.ProjectionHandle[P], error) {
	loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
	streamName := r.infra.storage.serverEvtStream.CachedInfo().Config.Name
	stream, err := r.infra.js.Stream(r.ctx, streamName)
	if err != nil {
		return events.ProjectionHandle[P]{}, fmt.Errorf("open EVT stream for %s: %w", name, err)
	}
	handle := evtstream.NewPreparedProjectionHandle(
		r.infra.js,
		stream,
		projection,
		r.logger.WithPrefix("core."+loggerName),
	)
	return registerProjectionHandle(
		r,
		handle,
		projection,
		key,
		name,
		streamName,
		evtstream.IdentityFromInfo,
		estimate,
		snapshotPolicy,
	), nil
}

func bindContentProjection[T any, P evtstream.ProjectionPointer[T]](
	view *ServerContentView,
	projection P,
	name string,
) (events.ProjectionHandle[P], error) {
	handle, err := evtstream.BindProjectionHandle(projection, view.projector)
	if err != nil {
		return events.ProjectionHandle[P]{}, fmt.Errorf("bind %s to ServerContentView: %w", name, err)
	}
	return handle, nil
}

func initializeCoreProjections(
	ctx context.Context,
	infra *coreInfrastructure,
	logger *log.Logger,
) (*coreProjections, error) {
	registrar := &projectionRegistrar{ctx: ctx, infra: infra, logger: logger}
	projections := &coreProjections{}

	roomDirectory := NewRoomDirectoryProjection()
	serverConfig := NewConfigProjection()
	roomGroupLayout := NewRoomGroupLayoutProjection()
	roomTimeline := NewRoomTimelineProjection()
	callState := NewCallStateProjection()
	assets := NewAssetProjection()
	threads := NewThreadProjection()
	reactions := NewReactionProjection()
	users := newUserProjectionWithDEKResolver(infra.dekResolver)
	userAuth := users.AuthProjection()
	contentKeys := NewContentKeyProjection()
	rbac := NewRBACProjection()
	mentionables := newMentionablesProjectionWithDEKResolver(infra.dekResolver)
	contentComponents := []events.SnapshotComponentModel{
		roomDirectory, serverConfig, roomGroupLayout, roomTimeline, callState,
		assets, threads, reactions, users, contentKeys, rbac, mentionables,
	}
	contentView := newServerContentView(
		newServerContentComponent(projectionsnapshot.ProjectionRoomDirectoryKey, roomDirectory, roomDirectory),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionServerConfigKey, serverConfig, serverConfig.Apply),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionRoomGroupLayoutKey, roomGroupLayout, roomGroupLayout.Apply),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionRoomTimelineKey, roomTimeline, roomTimeline.Apply),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionCallStateKey, callState, callState.Apply),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionAssetsKey, assets, assets.Apply),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionThreadsKey, threads, threads.Apply),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionReactionsKey, reactions, reactions.Apply),
		newServerContentComponent(projectionsnapshot.ProjectionUsersKey, users, users),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionContentKeysKey, contentKeys, contentKeys.Apply),
		newInfallibleServerContentComponent(projectionsnapshot.ProjectionRBACKey, rbac, rbac.Apply),
		newServerContentComponent(projectionsnapshot.ProjectionMentionablesKey, mentionables, mentionables),
	)
	contentHandle, err := registerPreparedProjection(
		registrar, contentView, projectionsnapshot.ProjectionServerContentViewKey,
		"Server Content View",
		func() (int64, int64, []ProjectionAdminMetric) {
			return contentView.adminProjectionEstimate(contentComponents...)
		},
		sharedSnapshots,
	)
	if err != nil {
		return nil, err
	}
	contentView.bindProjector(contentHandle.Projector())
	projections.contentView = contentView

	if projections.roomDirectory, err = bindContentProjection(contentView, roomDirectory, "room directory"); err != nil {
		return nil, err
	}
	if projections.serverConfig, err = bindContentProjection(contentView, serverConfig, "server config"); err != nil {
		return nil, err
	}
	if projections.roomGroupLayout, err = bindContentProjection(contentView, roomGroupLayout, "room group layout"); err != nil {
		return nil, err
	}
	if projections.roomTimeline, err = bindContentProjection(contentView, roomTimeline, "room timeline"); err != nil {
		return nil, err
	}
	if projections.callState, err = bindContentProjection(contentView, callState, "call state"); err != nil {
		return nil, err
	}
	if projections.assets, err = bindContentProjection(contentView, assets, "assets"); err != nil {
		return nil, err
	}
	if projections.threads, err = bindContentProjection(contentView, threads, "threads"); err != nil {
		return nil, err
	}
	if projections.reactions, err = bindContentProjection(contentView, reactions, "reactions"); err != nil {
		return nil, err
	}
	if projections.users, err = bindContentProjection(contentView, users, "users"); err != nil {
		return nil, err
	}
	if projections.contentKeys, err = bindContentProjection(contentView, contentKeys, "content keys"); err != nil {
		return nil, err
	}
	if projections.rbac, err = bindContentProjection(contentView, rbac, "RBAC"); err != nil {
		return nil, err
	}
	if projections.mentionables, err = bindContentProjection(contentView, mentionables, "mentionables"); err != nil {
		return nil, err
	}

	notificationDecisions := NewNotificationDecisionProjection()
	projections.notificationDecisions, err = registerProjection(
		registrar, notificationDecisions, projectionsnapshot.ProjectionNotificationDecisionsKey,
		"Notification Decisions", notificationDecisions.adminProjectionEstimate, sharedSnapshots,
	)
	if err != nil {
		return nil, err
	}

	notifications := NewNotificationProjection()
	notificationStreamName := infra.storage.notificationStream.CachedInfo().Config.Name
	notificationProjectionStream, err := infra.js.Stream(ctx, notificationStreamName)
	if err != nil {
		return nil, fmt.Errorf("open notification stream for projection: %w", err)
	}
	notificationHandle := notificationstream.NewProjectionHandle(
		infra.js, notificationProjectionStream, notifications,
		logger.WithPrefix("core.NotificationsProjector"),
	)
	projections.notifications = registerProjectionHandle(
		registrar, notificationHandle, notifications, projectionsnapshot.ProjectionNotificationsKey,
		"Notifications", notificationStreamName,
		notificationstream.IdentityFromInfo, notifications.adminProjectionEstimate, sharedSnapshots,
	)

	projections.userAuth, err = registerProjection(
		registrar, userAuth, "user_auth", "User Auth", userAuth.adminProjectionEstimate, coldReplayOnly,
	)
	if err != nil {
		return nil, err
	}

	invitations := NewInvitationProjection()
	projections.invitations, err = registerProjection(
		registrar,
		invitations,
		"invitations",
		"Invitations",
		invitations.adminProjectionEstimate,
		coldReplayOnly,
	)
	if err != nil {
		return nil, err
	}

	oauthClients := NewOAuthClientProjection()
	projections.oauthClients, err = registerProjection(
		registrar,
		oauthClients,
		"oauth_clients",
		"OAuth Clients",
		oauthClients.adminProjectionEstimate,
		coldReplayOnly,
	)
	if err != nil {
		return nil, err
	}

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
		componentized := registration.key == projectionsnapshot.ProjectionServerContentViewKey
		if componentized {
			source := projectionSnapshotCohortSource{repository: infra.snapshotRepository}
			if err := registration.projector.ConfigureSnapshotCohorts(
				registration.key, source, registration.identityResolver,
			); err != nil {
				return fmt.Errorf("configure %s projection snapshots: %w", registration.key, err)
			}
			projections.snapshotJobs = append(projections.snapshotJobs, projectionSnapshotJob{
				projector: registration.projector, repository: infra.snapshotRepository,
				projectionKey: registration.key, streamName: registration.streamName,
				componentized: true,
			})
			registration.snapshotEnabled = true
			continue
		}
		source := events.ProjectionSnapshotSource(projectionSnapshotSource{repository: infra.snapshotRepository})
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
		projections.snapshotJobs = append(projections.snapshotJobs, job)
		registration.snapshotEnabled = true
	}
	return nil
}
