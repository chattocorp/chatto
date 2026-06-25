import { describe, expect, it, vi } from 'vitest';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';

describe('ActiveCallRoomsState', () => {
	it('removes a failed local participant without hiding other active participants', () => {
		const state = new ActiveCallRoomsState(
			{ query: vi.fn() } as never,
			{ connected: false, roomId: null } as never
		);

		state.handleJoin('R1', 'call-1', {
			id: 'U1',
			displayName: 'Alice',
			login: 'alice',
			avatarUrl: null
		} as never);
		state.handleJoin('R1', 'call-1', {
			id: 'U2',
			displayName: 'Bob',
			login: 'bob',
			avatarUrl: null
		} as never);

		state.handleLeave('R1', 'call-1', 'U1');

		expect(state.has('R1')).toBe(true);
		expect(state.getParticipants('R1')).toEqual([
			{
				userId: 'U2',
				displayName: 'Bob',
				login: 'bob',
				avatarUrl: null
			}
		]);
	});

	it('reports backend-observed participants as voice call participants', () => {
		const state = new ActiveCallRoomsState(
			{ query: vi.fn() } as never,
			{ connected: false, roomId: null, participants: [] } as never
		);

		state.handleJoin('R1', 'call-1', {
			id: 'U1',
			displayName: 'Alice',
			login: 'alice',
			avatarUrl: null
		} as never);

		expect(state.getParticipantCallPresence('R1', 'U1')).toBe('voice');
		expect(state.getParticipantCallPresence('R1', 'U2')).toBeNull();
	});

	it('reports active LiveKit camera participants as video participants', () => {
		const state = new ActiveCallRoomsState(
			{ query: vi.fn() } as never,
			{
				connected: true,
				roomId: 'R1',
				participants: [
					{
						identity: 'U1',
						isCameraEnabled: true,
						videoTrack: {}
					},
					{
						identity: 'U2',
						isCameraEnabled: false,
						videoTrack: null
					}
				]
			} as never
		);

		expect(state.getParticipantCallPresence('R1', 'U1')).toBe('video');
		expect(state.getParticipantCallPresence('R1', 'U2')).toBe('voice');
		expect(state.getParticipantCallPresence('R2', 'U1')).toBeNull();
	});

	it('clears a room when its call ends', () => {
		const state = new ActiveCallRoomsState(
			{ query: vi.fn() } as never,
			{ connected: false, roomId: null } as never
		);

		state.handleJoin('R1', 'call-1', {
			id: 'U1',
			displayName: 'Alice',
			login: 'alice',
			avatarUrl: null
		} as never);

		expect(state.has('R1')).toBe(true);
		expect(state.getParticipants('R1')).toHaveLength(1);

		state.handleEnd('R1', 'call-1');

		expect(state.has('R1')).toBe(false);
		expect(state.getParticipants('R1')).toEqual([]);
	});

	it('clears a room with an unknown call id when a call end event arrives', async () => {
		const state = new ActiveCallRoomsState(
			{
				query: vi
					.fn()
					.mockReturnValueOnce({
						toPromise: vi.fn(async () => ({ data: { activeCallRoomIds: ['R1'] } }))
					})
					.mockReturnValueOnce({
						toPromise: vi.fn(async () => ({
							data: { room: { callParticipants: [] } }
						}))
					})
			} as never,
			{ connected: false, roomId: null } as never
		);

		await state.load();

		expect(state.has('R1')).toBe(true);

		state.handleEnd('R1', 'call-1');

		expect(state.has('R1')).toBe(false);
		expect(state.getParticipants('R1')).toEqual([]);
	});

	it('ignores stale leave and end events from an older call', () => {
		const state = new ActiveCallRoomsState(
			{ query: vi.fn() } as never,
			{ connected: false, roomId: null } as never
		);

		state.handleJoin('R1', 'call-2', {
			id: 'U1',
			displayName: 'Alice',
			login: 'alice',
			avatarUrl: null
		} as never);

		state.handleLeave('R1', 'call-1', 'U1');
		state.handleEnd('R1', 'call-1');

		expect(state.has('R1')).toBe(true);
		expect(state.getParticipants('R1')).toEqual([
			{
				userId: 'U1',
				displayName: 'Alice',
				login: 'alice',
				avatarUrl: null
			}
		]);
	});
});
