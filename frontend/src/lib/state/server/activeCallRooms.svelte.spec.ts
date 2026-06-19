import { describe, expect, it, vi } from 'vitest';
import {
	ActiveCallView,
	CallParticipantView,
	ListActiveCallsResponse
} from '$lib/pb/chatto/api/v1/chat_pb';
import { User } from '$lib/pb/chatto/core/v1/models_pb';
import { ActiveCallRoomsState } from './activeCallRooms.svelte';

describe('ActiveCallRoomsState', () => {
	function makeState(client = { listActiveCalls: vi.fn() }) {
		return new ActiveCallRoomsState(
			'server_1',
			{ connected: false, roomId: null } as never,
			() => client as never
		);
	}

	it('loads active calls and participants from the wire API', async () => {
		const client = {
			listActiveCalls: vi.fn(
				async () =>
					new ListActiveCallsResponse({
						calls: [
							new ActiveCallView({
								roomId: 'R1',
								callId: 'call-1',
								participants: [
									new CallParticipantView({
										callId: 'call-1',
										user: new User({
											id: 'U1',
											displayName: 'Alice',
											login: 'alice'
										})
									})
								]
							})
						]
					})
			)
		};
		const state = makeState(client);

		await state.load();

		expect(client.listActiveCalls).toHaveBeenCalledOnce();
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

	it('removes a failed local participant without hiding other active participants', () => {
		const state = makeState();

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
		const state = makeState();

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
		const state = makeState();

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
