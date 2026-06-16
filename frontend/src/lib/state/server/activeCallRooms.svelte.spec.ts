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
