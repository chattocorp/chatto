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

	roomDirectory            *RoomDirectoryProjection
	roomDirectoryProjector   *events.Projector
	serverConfig             *ConfigProjection
	serverConfigProjector    *events.Projector
	roomGroupLayout          *RoomGroupLayoutProjection
	roomGroupLayoutProjector *events.Projector
	roomTimeline             *RoomTimelineProjection
	roomTimelineProjector    *events.Projector
	callState                *CallStateProjection
	callStateProjector       *events.Projector
	assets                   *AssetProjection
	assetsProjector          *events.Projector
	threads                  *ThreadProjection
	threadsProjector         *events.Projector
	reactions                *ReactionProjection
	reactionsProjector       *events.Projector
	users                    *UserProjection
	usersProjector           *events.Projector
	userAuth                 *UserAuthProjection
	userAuthProjector        *events.Projector
	contentKeys              *ContentKeyProjection
	contentKeysProjector     *events.Projector
	rbac                     *RBACProjection
	rbacProjector            *events.Projector
	mentionables             *MentionablesProjection
	mentionablesProjector    *events.Projector
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

func (r *projectionRegistrar) register(
	projection events.Projection,
	key string,
	name string,
	estimate func() (int64, int64, []ProjectionAdminMetric),
	snapshotPolicy projectionSnapshotPolicy,
) *events.Projector {
	loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
	projector := events.NewProjector(
		r.infra.js,
		r.infra.storage.serverEvtStream,
		projection,
		r.logger.WithPrefix("core."+loggerName),
	)
	r.registrations = append(r.registrations, projectionRegistration{
		key:            key,
		name:           name,
		projector:      projector,
		subjects:       slices.Clone(projection.Subjects()),
		snapshotPolicy: snapshotPolicy,
		estimate:       estimate,
	})
	return projector
}

func initializeCoreProjections(
	infra *coreInfrastructure,
	logger *log.Logger,
) (*coreProjections, error) {
	registrar := &projectionRegistrar{infra: infra, logger: logger}
	projections := &coreProjections{}

	projections.roomDirectory = NewRoomDirectoryProjection()
	projections.roomDirectoryProjector = registrar.register(
		projections.roomDirectory,
		projectionsnapshot.ProjectionRoomDirectoryKey,
		"Room Directory",
		projections.roomDirectory.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.serverConfig = NewConfigProjection()
	projections.serverConfigProjector = registrar.register(
		projections.serverConfig,
		projectionsnapshot.ProjectionServerConfigKey,
		"Server Config",
		projections.serverConfig.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.roomGroupLayout = NewRoomGroupLayoutProjection()
	projections.roomGroupLayoutProjector = registrar.register(
		projections.roomGroupLayout,
		projectionsnapshot.ProjectionRoomGroupLayoutKey,
		"Room Group Layout",
		projections.roomGroupLayout.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.roomTimeline = NewRoomTimelineProjection()
	projections.roomTimelineProjector = registrar.register(
		projections.roomTimeline,
		projectionsnapshot.ProjectionRoomTimelineKey,
		"Room Timeline",
		projections.roomTimeline.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.callState = NewCallStateProjection()
	projections.callStateProjector = registrar.register(
		projections.callState,
		projectionsnapshot.ProjectionCallStateKey,
		"Call State",
		projections.callState.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.assets = NewAssetProjection()
	projections.assetsProjector = registrar.register(
		projections.assets,
		projectionsnapshot.ProjectionAssetsKey,
		"Assets",
		projections.assets.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.threads = NewThreadProjection()
	projections.threadsProjector = registrar.register(
		projections.threads,
		projectionsnapshot.ProjectionThreadsKey,
		"Threads",
		projections.threads.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.reactions = NewReactionProjection()
	projections.reactionsProjector = registrar.register(
		projections.reactions,
		projectionsnapshot.ProjectionReactionsKey,
		"Reactions",
		projections.reactions.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.users = newUserProjectionWithDEKResolver(infra.dekResolver)
	projections.usersProjector = registrar.register(
		projections.users,
		projectionsnapshot.ProjectionUsersKey,
		"Users",
		projections.users.adminProjectionEstimate,
		sharedSnapshots,
	)
	projections.userAuth = projections.users.AuthProjection()
	projections.userAuthProjector = registrar.register(
		projections.userAuth,
		"user_auth",
		"User Auth",
		projections.userAuth.adminProjectionEstimate,
		coldReplayOnly,
	)

	projections.contentKeys = NewContentKeyProjection()
	projections.contentKeysProjector = registrar.register(
		projections.contentKeys,
		projectionsnapshot.ProjectionContentKeysKey,
		"Content Keys",
		projections.contentKeys.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.rbac = NewRBACProjection()
	projections.rbacProjector = registrar.register(
		projections.rbac,
		projectionsnapshot.ProjectionRBACKey,
		"RBAC",
		projections.rbac.adminProjectionEstimate,
		sharedSnapshots,
	)

	projections.mentionables = newMentionablesProjectionWithDEKResolver(infra.dekResolver)
	projections.mentionablesProjector = registrar.register(
		projections.mentionables,
		projectionsnapshot.ProjectionMentionablesKey,
		"Mentionables",
		projections.mentionables.adminProjectionEstimate,
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
