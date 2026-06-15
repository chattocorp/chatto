import { describe, expect, it, vi } from 'vitest';
import { CallParticipantsState } from './callParticipants.svelte';

describe('CallParticipantsState', () => {
	it('removes a failed local participant from observer participants', async () => {
		const state = new CallParticipantsState({
			query: vi.fn(() => ({
				toPromise: vi.fn(async () => ({ data: { room: { callParticipants: [] } } }))
			}))
		} as never);

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
		const state = new CallParticipantsState({
			query: vi.fn(() => ({
				toPromise: vi.fn(async () => ({ data: { room: { callParticipants: [] } } }))
			}))
		} as never);

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
		const state = new CallParticipantsState({
			query: vi.fn(() => ({
				toPromise: vi.fn(async () => ({ data: { room: { callParticipants: [] } } }))
			}))
		} as never);

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
