import { render } from 'vitest-browser-svelte';
import { describe, expect, it } from 'vitest';
import DurableWorkersPanel from './DurableWorkersPanel.svelte';

describe('DurableWorkersPanel', () => {
	it('shows broker-derived worker health and queue state', () => {
		const { container } = render(DurableWorkersPanel, {
			props: {
				workers: [
					{
						key: 'user_key_shredding',
						health: 'working',
						pendingCount: '2',
						ackPendingCount: '1',
						waitingCount: '1',
						redeliveredCount: '3',
						lastDeliveredSequence: '44',
						ackFloorSequence: '41'
					}
				]
			}
		});

		expect(container.textContent).toContain('In progress');
		expect(container.textContent).toContain('user_key_shredding');
	});

	it('shows unavailable when an older server reports no worker diagnostics', () => {
		const { container } = render(DurableWorkersPanel, { props: { workers: [] } });
		expect(container.textContent).toContain('Unavailable');
	});

	it('labels ambiguous handler liveness as unconfirmed', () => {
		const { container } = render(DurableWorkersPanel, {
			props: {
				workers: [
					{
						key: 'asset_cleanup',
						health: 'unconfirmed',
						pendingCount: '0',
						ackPendingCount: '1',
						waitingCount: '0',
						redeliveredCount: '1',
						lastDeliveredSequence: '44',
						ackFloorSequence: '43'
					}
				]
			}
		});

		expect(container.textContent).toContain('Unconfirmed');
	});
});
