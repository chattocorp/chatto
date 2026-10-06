package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/log"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/logbridge"
	"hmans.de/chatto/internal/notificationstream"
	"hmans.de/chatto/internal/projectionsnapshot"
	"hmans.de/chatto/pkg/events"
)

// coreProjections is the complete construction result for core-owned
// projections. Its registration slice is the single source used by runtime
// lifecycle, readiness, and operator diagnostics.
type coreProjections struct {
	botWebhooks   events.ProjectionHandle[*botWebhookProjection]
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

// projectorOptions returns the construction options of a registered
// projection: its logger, consumer identity, and snapshot restore when the
// repository exists and the policy allows it. A componentized projection, such
// as ServerContentView, keeps its components in cohort storage. A
// single-payload projection keeps its payload in single-generation storage.
func (r *projectionRegistrar) projectorOptions(
	projection events.SubjectProjection,
	key string,
	name string,
	identityResolver events.StreamIdentityResolver,
	snapshotPolicy projectionSnapshotPolicy,
) events.ProjectorOptions {
	loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
	opts := events.ProjectorOptions{
		Logger:              logbridge.Slog(r.logger.WithPrefix("core." + loggerName)),
		ConsumerName:        key,
		ConsumerDescription: name,
	}
	if r.infra.snapshotRepository == nil || snapshotPolicy == coldReplayOnly {
		return opts
	}
	var source events.ProjectionSnapshotSource = projectionSnapshotSource{repository: r.infra.snapshotRepository}
	if _, componentized := projection.(events.ComponentSnapshotProjection); componentized {
		source = projectionSnapshotCohortSource{repository: r.infra.snapshotRepository}
	}
	opts.Snapshots = &events.SnapshotOptions{Key: key, Source: source, ResolveStreamIdentity: identityResolver}
	return opts
}

// register records a constructed projector for lifecycle, readiness,
// diagnostics, and snapshot publication.
func (r *projectionRegistrar) register(
	projector *events.Projector,
	projection events.SubjectProjection,
	key string,
	name string,
	streamName string,
	identityResolver events.StreamIdentityResolver,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
	opts events.ProjectorOptions,
) {
	_, componentSnapshots := projection.(events.ComponentSnapshotProjection)
	r.registrations = append(r.registrations, projectionRegistration{
		key:                key,
		name:               name,
		projector:          projector,
		subjects:           slices.Clone(projection.Subjects()),
		snapshotPolicy:     snapshotPolicy,
		snapshotEnabled:    opts.Snapshots != nil,
		componentSnapshots: componentSnapshots,
		streamName:         streamName,
		identityResolver:   identityResolver,
		estimate:           estimate,
	})
}

func registerProjection[T any, P evtstream.ProjectionPointer[T]](
	r *projectionRegistrar,
	projection P,
	key string,
	name string,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
) (events.ProjectionHandle[P], error) {
	streamName := r.infra.storage.serverEvtStream.CachedInfo().Config.Name
	stream, err := r.infra.js.Stream(r.ctx, streamName)
	if err != nil {
		return events.ProjectionHandle[P]{}, fmt.Errorf("open EVT stream for %s: %w", name, err)
	}
	opts := r.projectorOptions(projection, key, name, evtstream.IdentityFromInfo, snapshotPolicy)
	handle, err := evtstream.NewProjectionHandle(r.infra.js, stream, projection, opts)
	if err != nil {
		return events.ProjectionHandle[P]{}, fmt.Errorf("construct %s projector: %w", key, err)
	}
	r.register(handle.Projector(), projection, key, name, streamName, evtstream.IdentityFromInfo, estimate, snapshotPolicy, opts)
	return handle, nil
}

func registerPreparedProjection[T any, P evtstream.PreparedProjectionPointer[T]](
	r *projectionRegistrar,
	projection P,
	key string,
	name string,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
) (events.ProjectionHandle[P], error) {
	streamName := r.infra.storage.serverEvtStream.CachedInfo().Config.Name
	stream, err := r.infra.js.Stream(r.ctx, streamName)
	if err != nil {
		return events.ProjectionHandle[P]{}, fmt.Errorf("open EVT stream for %s: %w", name, err)
	}
	opts := r.projectorOptions(projection, key, name, evtstream.IdentityFromInfo, snapshotPolicy)
	handle, err := evtstream.NewPreparedProjectionHandle(r.infra.js, stream, projection, opts)
	if err != nil {
		return events.ProjectionHandle[P]{}, fmt.Errorf("construct %s projector: %w", key, err)
	}
	r.register(handle.Projector(), projection, key, name, streamName, evtstream.IdentityFromInfo, estimate, snapshotPolicy, opts)
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
	// The room timeline, thread, and reaction components and the Notification
	// Decisions projection index the same message IDs. One shared table holds
	// each ID once for all of them.
	eventIDs := newEventIDTable()
	roomTimeline := newRoomTimelineProjection(eventIDs)
	callState := NewCallStateProjection()
	assets := NewAssetProjection()
	threads := newThreadProjection(eventIDs)
	reactions := newReactionProjection(eventIDs)
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
			return contentView.adminProjectionEstimate(eventIDs, contentComponents...)
		},
		sharedSnapshots,
	)
	if err != nil {
		return nil, err
	}
	contentView.bindProjector(contentHandle.Projector())
	projections.contentView = contentView

	projections.roomDirectory = evtstream.BindProjectionHandle(roomDirectory, contentView.projector)
	projections.serverConfig = evtstream.BindProjectionHandle(serverConfig, contentView.projector)
	projections.roomGroupLayout = evtstream.BindProjectionHandle(roomGroupLayout, contentView.projector)
	projections.roomTimeline = evtstream.BindProjectionHandle(roomTimeline, contentView.projector)
	projections.callState = evtstream.BindProjectionHandle(callState, contentView.projector)
	projections.assets = evtstream.BindProjectionHandle(assets, contentView.projector)
	projections.threads = evtstream.BindProjectionHandle(threads, contentView.projector)
	projections.reactions = evtstream.BindProjectionHandle(reactions, contentView.projector)
	projections.users = evtstream.BindProjectionHandle(users, contentView.projector)
	projections.contentKeys = evtstream.BindProjectionHandle(contentKeys, contentView.projector)
	projections.rbac = evtstream.BindProjectionHandle(rbac, contentView.projector)
	projections.mentionables = evtstream.BindProjectionHandle(mentionables, contentView.projector)

	// Notification Decisions indexes the same message IDs as the content view,
	// so it interns them in the same process-wide table.
	notificationDecisions := newNotificationDecisionProjection(eventIDs)
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
	notificationOpts := registrar.projectorOptions(
		notifications, projectionsnapshot.ProjectionNotificationsKey, "Notifications",
		notificationstream.IdentityFromInfo, sharedSnapshots,
	)
	projections.notifications, err = notificationstream.NewProjectionHandle(
		infra.js, notificationProjectionStream, notifications, notificationOpts,
	)
	if err != nil {
		return nil, fmt.Errorf("construct %s projector: %w", projectionsnapshot.ProjectionNotificationsKey, err)
	}
	registrar.register(
		projections.notifications.Projector(), notifications, projectionsnapshot.ProjectionNotificationsKey,
		"Notifications", notificationStreamName, notificationstream.IdentityFromInfo,
		notifications.adminProjectionEstimate, sharedSnapshots, notificationOpts,
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

	webhooks := newBotWebhookProjection()
	projections.botWebhooks, err = registerProjection(registrar, webhooks, "bot_webhooks", "Bot Webhooks", webhooks.estimate, coldReplayOnly)
	if err != nil {
		return nil, err
	}
	projections.registrations = registrar.registrations
	projections.snapshotJobs = projectionSnapshotJobs(infra.snapshotRepository, projections.registrations)
	return projections, nil
}

// projectionSnapshotJobs returns one snapshot publication job for each
// registered projection that restores from snapshots.
func projectionSnapshotJobs(repository *projectionsnapshot.Repository, registrations []projectionRegistration) []projectionSnapshotJob {
	var jobs []projectionSnapshotJob
	for _, registration := range registrations {
		if !registration.snapshotEnabled {
			continue
		}
		jobs = append(jobs, projectionSnapshotJob{
			projector:     registration.projector,
			repository:    repository,
			projectionKey: registration.key,
			streamName:    registration.streamName,
			componentized: registration.componentSnapshots,
		})
	}
	return jobs
}
