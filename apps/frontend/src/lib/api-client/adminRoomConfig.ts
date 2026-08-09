import { AdminRoomConfigService } from '@chatto/api-types/admin/v1/room_config_connect';
import {
	protobufDurationToSeconds,
	secondsToProtobufDuration,
	type ProtobufDuration
} from '$lib/utils/protobufDuration';
import { authHeaders, createChattoClient, handleAuthError } from './connect.js';

export type AdminRoomConfigAPIConfig = {
	serverId?: string;
	baseUrl: string;
	bearerToken: string | null;
	onAuthenticationRequired?: (serverId: string) => void;
};

export type RoomConfigScope =
	| { kind: 'server' }
	| { kind: 'room-group'; id: string }
	| { kind: 'room'; id: string };

export type RoomConfigState = {
	authorEditWindowSeconds: number | null;
	effectiveAuthorEditWindowSeconds: number;
};

function apiScope(scope: RoomConfigScope) {
	switch (scope.kind) {
		case 'server':
			return { scope: { case: 'server' as const, value: true } };
		case 'room-group':
			return { scope: { case: 'roomGroupId' as const, value: scope.id } };
		case 'room':
			return { scope: { case: 'roomId' as const, value: scope.id } };
	}
}

function mapState(
	state:
		| {
				layer?: { authorEditWindow?: ProtobufDuration };
				effective?: { authorEditWindow?: ProtobufDuration };
		  }
		| undefined
): RoomConfigState {
	return {
		authorEditWindowSeconds: protobufDurationToSeconds(state?.layer?.authorEditWindow) ?? null,
		effectiveAuthorEditWindowSeconds:
			protobufDurationToSeconds(state?.effective?.authorEditWindow) ?? 3 * 60 * 60
	};
}

export function createAdminRoomConfigAPI(config: AdminRoomConfigAPIConfig) {
	const roomConfig = createChattoClient(AdminRoomConfigService, config);
	const headers = () => authHeaders(config);
	return {
		async getRoomConfig(
			scope: RoomConfigScope,
			options: { signal?: AbortSignal } = {}
		): Promise<RoomConfigState> {
			try {
				const response = await roomConfig.getRoomConfig(
					{ scope: apiScope(scope) },
					{ headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
				);
				return mapState(response.state);
			} catch (error) {
				return handleAuthError(config, error);
			}
		},

		async updateRoomConfig(
			scope: RoomConfigScope,
			authorEditWindowSeconds: number | null
		): Promise<RoomConfigState> {
			try {
				const response = await roomConfig.updateRoomConfig(
					{
						scope: apiScope(scope),
						config: {
							authorEditWindow:
								authorEditWindowSeconds == null
									? undefined
									: secondsToProtobufDuration(authorEditWindowSeconds)
						},
						updateMask: { paths: ['author_edit_window'] }
					},
					{ headers: headers() }
				);
				return mapState(response.state);
			} catch (error) {
				return handleAuthError(config, error);
			}
		}
	};
}
