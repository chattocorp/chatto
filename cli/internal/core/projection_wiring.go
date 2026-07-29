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
	userAuthProjector        *events.Projector
	contentKeys              *ContentKeyProjection
	contentKeysProjector     *events.Projector
	rbac                     *RBACProjection
	rbacProjector            *events.Projector
	mentionables             *MentionablesProjection
	mentionablesProjector    *events.Projector
}

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
) *events.Projector {
	loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
	projector := events.NewProjector(
		r.infra.js,
		r.infra.storage.serverEvtStream,
		projection,
		r.logger.WithPrefix("core."+loggerName),
	)
	r.registrations = append(r.registrations, projectionRegistration{
		key:       key,
		name:      name,
		projector: projector,
		subjects:  slices.Clone(projection.Subjects()),
		estimate:  estimate,
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
		"room_directory",
		"Room Directory",
		projections.roomDirectory.adminProjectionEstimate,
	)

	projections.serverConfig = NewConfigProjection()
	projections.serverConfigProjector = registrar.register(
		projections.serverConfig,
		"server_config",
		"Server Config",
		projections.serverConfig.adminProjectionEstimate,
	)

	projections.roomGroupLayout = NewRoomGroupLayoutProjection()
	projections.roomGroupLayoutProjector = registrar.register(
		projections.roomGroupLayout,
		"room_group_layout",
		"Room Group Layout",
		projections.roomGroupLayout.adminProjectionEstimate,
	)

	projections.roomTimeline = NewRoomTimelineProjection()
	projections.roomTimelineProjector = registrar.register(
		projections.roomTimeline,
		"room_timeline",
		"Room Timeline",
		projections.roomTimeline.adminProjectionEstimate,
	)

	projections.callState = NewCallStateProjection()
	projections.callStateProjector = registrar.register(
		projections.callState,
		"call_state",
		"Call State",
		projections.callState.adminProjectionEstimate,
	)

	projections.assets = NewAssetProjection()
	projections.assetsProjector = registrar.register(
		projections.assets,
		"assets",
		"Assets",
		projections.assets.adminProjectionEstimate,
	)

	projections.threads = NewThreadProjection()
	projections.threadsProjector = registrar.register(
		projections.threads,
		"threads",
		"Threads",
		projections.threads.adminProjectionEstimate,
	)

	projections.reactions = NewReactionProjection()
	projections.reactionsProjector = registrar.register(
		projections.reactions,
		"reactions",
		"Reactions",
		projections.reactions.adminProjectionEstimate,
	)

	projections.users = newUserProjectionWithDEKResolver(infra.dekResolver)
	projections.usersProjector = registrar.register(
		projections.users,
		"users",
		"Users",
		projections.users.adminProjectionEstimate,
	)
	userAuth := projections.users.AuthProjection()
	projections.userAuthProjector = registrar.register(
		userAuth,
		"user_auth",
		"User Auth",
		userAuth.adminProjectionEstimate,
	)

	projections.contentKeys = NewContentKeyProjection()
	projections.contentKeysProjector = registrar.register(
		projections.contentKeys,
		"content_keys",
		"Content Keys",
		projections.contentKeys.adminProjectionEstimate,
	)

	projections.rbac = NewRBACProjection()
	projections.rbacProjector = registrar.register(
		projections.rbac,
		"rbac",
		"RBAC",
		projections.rbac.adminProjectionEstimate,
	)

	projections.mentionables = newMentionablesProjectionWithDEKResolver(infra.dekResolver)
	projections.mentionablesProjector = registrar.register(
		projections.mentionables,
		"mentionables",
		"Mentionables",
		projections.mentionables.adminProjectionEstimate,
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
	specs := []struct {
		key       string
		projector *events.Projector
	}{
		{projectionsnapshot.ProjectionThreadsKey, projections.threadsProjector},
		{projectionsnapshot.ProjectionRoomDirectoryKey, projections.roomDirectoryProjector},
		{projectionsnapshot.ProjectionServerConfigKey, projections.serverConfigProjector},
		{projectionsnapshot.ProjectionRoomGroupLayoutKey, projections.roomGroupLayoutProjector},
		{projectionsnapshot.ProjectionRoomTimelineKey, projections.roomTimelineProjector},
		{projectionsnapshot.ProjectionCallStateKey, projections.callStateProjector},
		{projectionsnapshot.ProjectionAssetsKey, projections.assetsProjector},
		{projectionsnapshot.ProjectionReactionsKey, projections.reactionsProjector},
		{projectionsnapshot.ProjectionContentKeysKey, projections.contentKeysProjector},
		{projectionsnapshot.ProjectionRBACKey, projections.rbacProjector},
		{projectionsnapshot.ProjectionMentionablesKey, projections.mentionablesProjector},
		{projectionsnapshot.ProjectionUsersKey, projections.usersProjector},
	}

	for _, spec := range specs {
		if err := spec.projector.ConfigureSnapshots(
			spec.key,
			projectionSnapshotSource{repository: infra.snapshotRepository},
			infra.snapshotStreamIdentity,
		); err != nil {
			return fmt.Errorf("configure %s projection snapshots: %w", spec.key, err)
		}
		projections.snapshotJobs = append(projections.snapshotJobs, projectionSnapshotJob{
			projector:      spec.projector,
			repository:     infra.snapshotRepository,
			projectionKey:  spec.key,
			streamName:     streamName,
			streamIdentity: infra.snapshotStreamIdentity,
		})
		for i := range projections.registrations {
			if projections.registrations[i].key == spec.key {
				projections.registrations[i].snapshotEnabled = true
				break
			}
		}
	}
	return nil
}
