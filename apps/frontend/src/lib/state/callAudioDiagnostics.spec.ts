import { EventEmitter } from 'node:events';
import type { Room } from 'livekit-client';
import { afterEach, beforeEach, expect, it, vi, type MockInstance } from 'vitest';
import { CallAudioDiagnostics } from './callAudioDiagnostics';

/** SDK-shaped fixture with private sentinel fields to test the logging boundary. */
function fixture() {
  const element = {
    paused: false,
    muted: false,
    volume: 1,
    readyState: 4,
    error: null,
    srcObject: { private: 'SECRET-stream' },
    sinkId: 'SECRET-device'
  };
  const stats = {
    type: 'inbound-rtp',
    kind: 'audio',
    packetsReceived: 200,
    bytesReceived: 3000,
    totalSamplesReceived: 9600,
    totalAudioEnergy: 0.5,
    jitter: 0.01,
    id: 'SECRET-stats',
    timestamp: 200_000,
    lastPacketReceivedTimestamp: 70_000,
    trackIdentifier: 'SECRET-track',
    address: 'SECRET-address',
    packetsLost: 'SECRET-invalid-number'
  };
  const track = {
    receiver: {},
    streamState: 'active',
    mediaStreamTrack: { id: 'SECRET-media', enabled: true, muted: false, readyState: 'live' },
    attachedElements: [element],
    getRTCStatsReport: vi.fn(async () => new Map([['SECRET-key', stats]]))
  };
  const publication = {
    kind: 'audio',
    source: 'microphone',
    trackSid: 'SECRET-publication',
    isSubscribed: true,
    isDesired: true,
    permissionStatus: 'allowed',
    isEnabled: true,
    isMuted: false,
    track,
    setEnabled: vi.fn(),
    setSubscribed: vi.fn()
  };
  const participant = {
    identity: 'SECRET-account',
    sid: 'SECRET-session',
    name: 'SECRET-name',
    metadata: 'SECRET-metadata',
    trackPublications: new Map([[publication.trackSid, publication]])
  };
  const room = Object.assign(new EventEmitter(), {
    canPlaybackAudio: true,
    remoteParticipants: new Map([[participant.identity, participant]])
  });
  // The fixture implements only the Room surface read by diagnostics; no network is opened.
  const diagnostics = new CallAudioDiagnostics(room as unknown as Room);
  return { room, diagnostics, participant, publication, track, element, stats };
}

let output: MockInstance<typeof console.debug>;
beforeEach(() => {
  vi.useFakeTimers();
  output = vi.spyOn(console, 'debug').mockImplementation(() => {});
});
afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function entries(): Record<string, unknown>[] {
  return output.mock.calls.map((call) => JSON.parse(call[1] as string) as Record<string, unknown>);
}

it('logs only allowlisted state and counters, correlating events without private fields', async () => {
  const f = fixture();
  try {
    f.room.emit('trackSubscribed', f.track, f.publication, f.participant);
    f.room.emit(
      'encryptionError',
      Object.assign(new Error('SECRET-error'), {
        reason: 1,
        participantIdentity: 'SECRET-account',
        stack: 'SECRET-stack'
      }),
      f.participant
    );
    f.room.emit('trackSubscriptionFailed', f.publication.trackSid, f.participant, 1);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(entries()).toContainEqual(
      expect.objectContaining({
        event: 'trackSubscribed',
        participant: 1,
        publication: 1,
        receiver: 1
      })
    );
    expect(entries()).toContainEqual(
      expect.objectContaining({
        event: 'audio-rtp',
        participant: 1,
        publication: 1,
        packetsReceived: 200,
        totalSamplesReceived: 9600,
        rtpStream: 1,
        lastPacketAgeMs: 130_000
      })
    );
    expect(entries()).toContainEqual(
      expect.objectContaining({ event: 'encryption-error', reason: 'missing-key' })
    );
    expect(JSON.stringify(output.mock.calls)).not.toContain('SECRET');
    expect(f.publication.setEnabled).not.toHaveBeenCalled();
    expect(f.publication.setSubscribed).not.toHaveBeenCalled();
  } finally {
    f.diagnostics.dispose();
  }
});

it('distinguishes missing subscriptions, disabled delivery and paused playback', async () => {
  const f = fixture();
  try {
    f.publication.isEnabled = false;
    f.element.paused = true;
    await vi.advanceTimersByTimeAsync(10_000);
    expect(entries()).toContainEqual(
      expect.objectContaining({
        event: 'audio-state',
        enabled: false,
        elements: [expect.objectContaining({ paused: true })]
      })
    );
    f.publication.isSubscribed = false;
    Reflect.set(f.publication, 'track', undefined);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(entries()).toContainEqual(
      expect.objectContaining({
        event: 'audio-state',
        subscribed: false,
        hasTrack: false
      })
    );
  } finally {
    f.diagnostics.dispose();
  }
});

it('ignores pending stats after track replacement and does not overlap reads', async () => {
  const f = fixture();
  let resolve!: (value: Map<string, typeof f.stats>) => void;
  f.track.getRTCStatsReport.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    })
  );
  try {
    await vi.advanceTimersByTimeAsync(20_000);
    expect(f.track.getRTCStatsReport).toHaveBeenCalledTimes(1);
    Reflect.set(f.publication, 'track', { ...f.track, receiver: {} });
    resolve(new Map([['old', f.stats]]));
    await Promise.resolve();
    expect(entries().filter((entry) => entry.event === 'audio-rtp')).toHaveLength(0);
  } finally {
    f.diagnostics.dispose();
  }
});

it('removes listeners and timers and discards late results on disposal', async () => {
  const f = fixture();
  let resolve!: (value: Map<string, typeof f.stats>) => void;
  f.track.getRTCStatsReport.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    })
  );
  await vi.advanceTimersByTimeAsync(10_000);
  f.diagnostics.dispose();
  f.diagnostics.dispose();
  const count = output.mock.calls.length;
  resolve(new Map([['late', f.stats]]));
  f.room.emit('participantConnected', f.participant);
  await vi.advanceTimersByTimeAsync(20_000);
  expect(output).toHaveBeenCalledTimes(count);
  expect(f.room.eventNames()).toEqual([]);
  expect(vi.getTimerCount()).toBe(0);
});

it('bounds repeated errors and reports their accumulated count', async () => {
  const f = fixture();
  try {
    for (let i = 0; i < 100; i++)
      f.room.emit('encryptionError', new Error('SECRET-error'), f.participant);
    expect(entries().filter((entry) => entry.event === 'encryption-error')).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(10_000);
    f.room.emit('encryptionError', new Error('SECRET-error'), f.participant);
    expect(entries()).toContainEqual(
      expect.objectContaining({ event: 'encryption-error', occurrences: 101 })
    );
  } finally {
    f.diagnostics.dispose();
  }
});

it('correlates participant rejoins and receiver reuse without exposing IDs', () => {
  const f = fixture();
  try {
    f.room.emit('trackSubscribed', f.track, f.publication, f.participant);
    const replacement = { ...f.participant, sid: 'SECRET-new-session' };
    const publication = { ...f.publication, trackSid: 'SECRET-new-publication' };
    f.room.emit('trackSubscribed', f.track, publication, replacement);
    expect(entries()).toContainEqual(
      expect.objectContaining({
        event: 'trackSubscribed',
        participant: 1,
        session: 2,
        publication: 2,
        receiver: 1
      })
    );
    expect(JSON.stringify(output.mock.calls)).not.toContain('SECRET');
  } finally {
    f.diagnostics.dispose();
  }
});

it('reports missing and failed stats without logging the raw error', async () => {
  const f = fixture();
  try {
    f.track.getRTCStatsReport.mockResolvedValueOnce(new Map());
    await vi.advanceTimersByTimeAsync(10_000);
    expect(entries()).toContainEqual(expect.objectContaining({ event: 'audio-rtp-unavailable' }));
    f.track.getRTCStatsReport.mockRejectedValueOnce(new Error('SECRET-token-in-error'));
    await vi.advanceTimersByTimeAsync(10_000);
    expect(entries()).toContainEqual(expect.objectContaining({ event: 'stats-failed' }));
    expect(JSON.stringify(output.mock.calls)).not.toContain('SECRET');
  } finally {
    f.diagnostics.dispose();
  }
});
