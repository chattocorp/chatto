import { AdminRoomConfigService } from '@chatto/api-types/admin/v1/room_config_connect';
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

export type RoomConfigSource = {
	kind: 'product-default' | 'server' | 'room-group' | 'room' | 'unknown';
	id: string | null;
};

export type RoomConfigState = {
	authorEditWindowSeconds: number | null;
	effectiveAuthorEditWindowSeconds: number;
	authorEditWindowSource: RoomConfigSource;
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

function mapSource(
	source:
		| {
				source?:
					| { case: 'productDefault'; value: boolean }
					| { case: 'server'; value: boolean }
					| { case: 'roomGroupId'; value: string }
					| { case: 'roomId'; value: string }
					| { case: undefined; value?: undefined };
		  }
		| undefined
): RoomConfigSource {
	switch (source?.source?.case) {
		case 'productDefault':
			return { kind: 'product-default', id: null };
		case 'server':
			return { kind: 'server', id: null };
		case 'roomGroupId':
			return { kind: 'room-group', id: source.source.value };
		case 'roomId':
			return { kind: 'room', id: source.source.value };
		default:
			return { kind: 'unknown', id: null };
	}
}

function mapState(
	state:
		| {
				layer?: { authorEditWindowSeconds?: number };
				effective?: { authorEditWindowSeconds?: number };
				sources?: { authorEditWindow?: Parameters<typeof mapSource>[0] };
		  }
		| undefined
): RoomConfigState {
	return {
		authorEditWindowSeconds: state?.layer?.authorEditWindowSeconds ?? null,
		effectiveAuthorEditWindowSeconds: state?.effective?.authorEditWindowSeconds ?? 3 * 60 * 60,
		authorEditWindowSource: mapSource(state?.sources?.authorEditWindow)
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
						layer: {
							authorEditWindowSeconds: authorEditWindowSeconds ?? undefined
						},
						updateMask: { paths: ['author_edit_window_seconds'] }
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
