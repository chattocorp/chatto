import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { AdminNatsStreamInfo } from '$lib/api-client/adminDiagnostics';
import JobQueuePanel from './JobQueuePanel.svelte';

const queue: AdminNatsStreamInfo = {
  name: 'JOBS',
  description: '',
  subjects: ['jobs.>'],
  storage: 'File',
  messages: 3,
  bytes: 2048,
  firstSequence: '1',
  lastSequence: '3',
  consumerCount: 1,
  replicas: 1,
  clusterLeader: '',
  oldestMessageAgeSeconds: 3600,
  maxAgeSeconds: 604800
};

describe('job queue summary', () => {
  it('shows retained work, sampled age, storage and retention without individual jobs', async () => {
    const screen = render(JobQueuePanel, { queue });
    await expect.element(screen.getByText('Outstanding jobs', { exact: true })).toBeVisible();
    await expect.element(screen.getByText('3', { exact: true })).toBeVisible();
    await expect.element(screen.getByText('1 hr', { exact: true })).toBeVisible();
    await expect.element(screen.getByText('2 KB', { exact: true })).toBeVisible();
    await expect.element(screen.getByText('7 days', { exact: true })).toBeVisible();
  });

  it('shows an empty queue without inventing an oldest age', async () => {
    const screen = render(JobQueuePanel, {
      queue: { ...queue, messages: 0, bytes: 0, oldestMessageAgeSeconds: null }
    });
    await expect.element(screen.getByText('0', { exact: true })).toBeVisible();
    await expect.element(screen.getByText('—', { exact: true })).toBeVisible();
  });

  it('shows missing telemetry as unavailable, rather than zero outstanding work', async () => {
    const screen = render(JobQueuePanel, { queue: undefined });
    await expect.element(screen.getByText('Unavailable', { exact: true })).toBeVisible();
    expect(screen.container.querySelector('[data-testid="job-queue-stats"]')).toBeNull();
  });
});
