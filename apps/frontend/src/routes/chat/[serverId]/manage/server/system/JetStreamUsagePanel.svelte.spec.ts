import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { AdminAccountInfo, AdminNatsStats } from '$lib/api/adminDiagnostics';
import JetStreamUsagePanel from './JetStreamUsagePanel.svelte';

const account: AdminAccountInfo = {
  memory: -1,
  memoryUsed: 0,
  storage: 1000,
  storageUsed: 1200,
  streams: -1,
  streamsUsed: 2,
  consumers: -1,
  consumersUsed: 2
};

function stream(name: string, storage: string, replicas: number, bytes = 100) {
  return {
    name,
    description: '',
    subjects: [],
    storage,
    messages: 1,
    bytes,
    firstSequence: '1',
    lastSequence: '1',
    consumerCount: 0,
    replicas,
    clusterLeader: ''
  };
}

function consumer(pullBased: boolean, pushBound: boolean) {
  return {
    stream: 'EVT',
    name: `consumer-${pullBased}-${pushBound}`,
    durable: '',
    filterSubject: '',
    filterSubjects: [],
    ackPolicy: 'explicit',
    pullBased,
    pushBound,
    pending: 0,
    ackPending: 0,
    redelivered: 0,
    waiting: 0,
    deliveredConsumerSequence: '0',
    deliveredStreamSequence: '0',
    ackFloorConsumerSequence: '0',
    ackFloorStreamSequence: '0'
  };
}

function nats(overrides: Partial<AdminNatsStats> = {}): AdminNatsStats {
  return {
    totalMessages: 2,
    totalBytes: 200,
    totalConsumerPending: 0,
    totalAckPending: 0,
    streams: [stream('EVT', 'File', 1), stream('PRESENCE', 'Memory', 1)],
    consumers: [consumer(true, false), consumer(false, true)],
    ...overrides
  };
}

function renderPanel(natsStats: AdminNatsStats) {
  return render(JetStreamUsagePanel, {
    props: { account, accountAvailable: true, nats: natsStats, natsAvailable: true }
  });
}

describe('JetStreamUsagePanel', () => {
  it('shows a meter only for limited resources and clamps its value to the limit', () => {
    const { container } = renderPanel(nats());
    const meters = container.querySelectorAll('[role="meter"]');

    expect(meters).toHaveLength(1);
    expect(meters[0].getAttribute('aria-label')).toBe('File Storage');
    expect(meters[0].getAttribute('aria-valuenow')).toBe('1000');
    expect(container.textContent).toContain('no limit');
  });

  it('explains replica copies only when a file stream is replicated', () => {
    const replicatedMemory = renderPanel(
      nats({ streams: [stream('EVT', 'File', 1), stream('PRESENCE', 'Memory', 3)] })
    );
    expect(replicatedMemory.container.textContent).not.toContain('copy on each stream replica');
    replicatedMemory.unmount();

    const replicatedFile = renderPanel(nats({ streams: [stream('EVT', 'File', 3, 400)] }));
    expect(replicatedFile.container.textContent).toContain(
      'Includes a copy on each stream replica. The file streams hold 400 B of data.'
    );
  });

  it('warns about unbound push consumers', () => {
    const { container } = renderPanel(
      nats({ consumers: [consumer(true, false), consumer(false, false)] })
    );

    expect(container.textContent).toContain('1 pull, 1 push');
    expect(container.textContent).toContain('1 unbound');
  });
});
