package core

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/log"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/projectionsnapshot"
)

// coreProjections is the complete construction result for core-owned
// projections. Its registration slice is the single source used by runtime
// lifecycle, readiness, and operator diagnostics.
type coreProjections struct {
	registrations []projectionRegistration
	snapshotJobs  []projectionSnapshotJob

	roomDirectory   events.ProjectionHandle[*RoomDirectoryProjection]
	serverConfig    events.ProjectionHandle[*ConfigProjection]
	roomGroupLayout events.ProjectionHandle[*RoomGroupLayoutProjection]
	roomTimeline    events.ProjectionHandle[*RoomTimelineProjection]
	callState       events.ProjectionHandle[*CallStateProjection]
	assets          events.ProjectionHandle[*AssetProjection]
	threads         events.ProjectionHandle[*ThreadProjection]
	reactions       events.ProjectionHandle[*ReactionProjection]
	users           events.ProjectionHandle[*UserProjection]
	userAuth        events.ProjectionHandle[*UserAuthProjection]
	contentKeys     events.ProjectionHandle[*ContentKeyProjection]
	rbac            events.ProjectionHandle[*RBACProjection]
	mentionables    events.ProjectionHandle[*MentionablesProjection]
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

func registerProjection[T any, P events.ProjectionPointer[T]](
	r *projectionRegistrar,
	projection P,
	key string,
	name string,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
) events.ProjectionHandle[P] {
	loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
	handle := events.NewProjectionHandle(
		r.infra.js,
		r.infra.storage.serverEvtStream,
		projection,
		r.logger.WithPrefix("core."+loggerName),
	)
	r.registrations = append(r.registrations, projectionRegistration{
		key:            key,
		name:           name,
		projector:      handle.Projector(),
		subjects:       slices.Clone(projection.Subjects()),
		snapshotPolicy: snapshotPolicy,
		estimate:       estimate,
	})
	return handle
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

	streamName := infra.storage.serverEvtStream.CachedInfo().Config.Name
	for i := range projections.registrations {
		registration := &projections.registrations[i]
		if registration.snapshotPolicy == coldReplayOnly {
			continue
		}
		if err := registration.projector.ConfigureSnapshots(
			registration.key,
			projectionSnapshotSource{repository: infra.snapshotRepository},
			infra.snapshotStreamIdentity,
		); err != nil {
			return fmt.Errorf("configure %s projection snapshots: %w", registration.key, err)
		}
		projections.snapshotJobs = append(projections.snapshotJobs, projectionSnapshotJob{
			projector:      registration.projector,
			repository:     infra.snapshotRepository,
			projectionKey:  registration.key,
			streamName:     streamName,
			streamIdentity: infra.snapshotStreamIdentity,
		})
		registration.snapshotEnabled = true
	}
	return nil
}
