import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { AdminAssetCleanupStatus } from '$lib/api-client/adminDiagnostics';
import AssetCleanupPanel from './AssetCleanupPanel.svelte';

function status(overrides: Partial<AdminAssetCleanupStatus> = {}): AdminAssetCleanupStatus {
  return {
    available: true,
    health: 'healthy',
    pendingCount: 0,
    oldestPendingAt: null,
    passInProgress: false,
    lastPassAt: new Date('2026-07-10T11:00:00Z'),
    lastSuccessfulPassAt: new Date('2026-07-10T11:00:00Z'),
    updatedAt: new Date('2026-07-10T11:00:05Z'),
    lastPassFailed: false,
    lastInspectedSequence: '44',
    latestDeletionSequence: '44',
    ...overrides
  };
}

describe('AssetCleanupPanel', () => {
  it('shows healthy caught-up cleanup without pending retries', () => {
    const { container } = render(AssetCleanupPanel, { props: { status: status() } });

    expect(container.textContent).toContain('Asset cleanup');
    expect(container.textContent).toContain('Healthy');
    expect(container.textContent).toContain('All known physical deletion work is complete.');
    expect(container.textContent).toContain('Caught up');
  });

  it('shows retry backlog and an event scan that is behind', () => {
    const { container } = render(AssetCleanupPanel, {
      props: {
        status: status({
          health: 'retrying',
          pendingCount: 2,
          oldestPendingAt: new Date('2026-07-10T10:00:00Z'),
          lastPassFailed: true,
          lastInspectedSequence: '41'
        })
      }
    });

    expect(container.textContent).toContain('Retrying');
    expect(container.textContent).toContain('One or more deletions will be retried automatically.');
    expect(container.textContent).toContain('2');
    expect(container.textContent).toContain('New events waiting');
  });

  it('shows unavailable state for an older server without diagnostics', () => {
    const { container } = render(AssetCleanupPanel, {
      props: { status: status({ available: false, health: 'unavailable', lastPassAt: null }) }
    });

    expect(container.textContent).toContain('Unavailable');
    expect(container.textContent).toContain(
      'Cleanup health is not reported by this server version.'
    );
    expect(container.textContent).not.toContain('Caught up');
    expect(container.textContent).not.toContain('Never');
    expect(container.textContent).not.toContain('None');
  });

  it('shows a stalled worker as an operational failure', () => {
    const { container } = render(AssetCleanupPanel, {
      props: { status: status({ health: 'stalled', lastPassFailed: true }) }
    });

    expect(container.textContent).toContain('Stalled');
    expect(container.textContent).toContain('The elected worker has stopped reporting progress.');
  });
});
