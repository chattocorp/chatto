import { describe, expect, it, vi } from 'vitest';
import {
	CallParticipantView,
	GetCallParticipantsResponse
} from '$lib/pb/chatto/api/v1/chat_pb';
import { User } from '$lib/pb/chatto/core/v1/models_pb';
import { CallParticipantsState } from './callParticipants.svelte';

describe('CallParticipantsState', () => {
	function makeState(client = { getCallParticipants: vi.fn(async () => new GetCallParticipantsResponse()) }) {
		return new CallParticipantsState('server_1', () => client as never);
	}

	it('loads call participants from the wire API', async () => {
		const client = {
			getCallParticipants: vi.fn(
				async () =>
					new GetCallParticipantsResponse({
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
			)
		};
		const state = makeState(client);

		await state.load('R1');

		expect(client.getCallParticipants).toHaveBeenCalledOnce();
		expect(state.participants).toEqual([
			{
				userId: 'U1',
				displayName: 'Alice',
				login: 'alice',
				avatarUrl: null
			}
		]);
	});

	it('removes a failed local participant from observer participants', async () => {
		const state = makeState();
		await state.load('R1');
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

		expect(state.participants).toEqual([
			{
				userId: 'U2',
				displayName: 'Bob',
				login: 'bob',
				avatarUrl: null
			}
		]);
	});

	it('clears observer participants when the current room call ends', async () => {
		const state = makeState();

		await state.load('R1');
		state.handleJoin('R1', 'call-1', {
			id: 'U1',
			displayName: 'Alice',
			login: 'alice',
			avatarUrl: null
		} as never);

		expect(state.participants).toHaveLength(1);

		state.handleEnd('R1', 'call-1');

		expect(state.participants).toEqual([]);
	});

	it('ignores stale leave and end events from an older call', async () => {
		const state = makeState();

		await state.load('R1');
		state.handleJoin('R1', 'call-2', {
			id: 'U1',
			displayName: 'Alice',
			login: 'alice',
			avatarUrl: null
		} as never);

		state.handleLeave('R1', 'call-1', 'U1');
		state.handleEnd('R1', 'call-1');

		expect(state.participants).toEqual([
			{
				userId: 'U1',
				displayName: 'Alice',
				login: 'alice',
				avatarUrl: null
			}
		]);
	});
});
