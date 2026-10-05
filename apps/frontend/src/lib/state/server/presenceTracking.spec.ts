import { afterEach, describe, expect, it, vi } from 'vitest';
import { PresencePreference, PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  initPresenceTracking,
  refreshPresencePreference,
  setPresenceStatus,
  type PresenceReporter
} from './presenceTracking';

const scope = { serverId: 'origin', userId: 'user' };

function reporter(): PresenceReporter {
  let saved = new PresencePreference({ status: PresenceStatus.ONLINE, revision: 'one' });
  return {
    ...scope,
    getPreference: vi.fn(async () => saved),
    setPreference: vi.fn(async (status: PresenceStatus, revision: string) => {
      saved = new PresencePreference({ status, revision: `${revision}-next` });
      return saved;
    }),
    refreshPresence: vi.fn(async () => saved)
  };
}

async function settle() {
  for (let i = 0; i < 12; i++) await Promise.resolve();
}

describe('frontend presence tracking', () => {
  let stop: (() => void) | undefined;
  afterEach(() => {
    stop?.();
    stop = undefined;
  });

  it('rejects a selection while no chat root tracks presence', async () => {
    await expect(setPresenceStatus(scope, PresenceStatus.AWAY)).rejects.toThrow('not connected');
    expect(() => refreshPresencePreference(scope)).not.toThrow();
  });

  it('selects through the active tracker until it stops', async () => {
    const api = reporter();
    const tracking = initPresenceTracking(() => [api]);
    stop = tracking.stop;
    tracking.sync();
    await settle();

    await setPresenceStatus(scope, PresenceStatus.AWAY);
    expect(api.setPreference).toHaveBeenLastCalledWith(PresenceStatus.AWAY, expect.any(String));

    tracking.stop();
    stop = undefined;
    await expect(setPresenceStatus(scope, PresenceStatus.ONLINE)).rejects.toThrow('not connected');
  });

  it('keeps the newer tracker active when an older one stops', async () => {
    const api = reporter();
    const older = initPresenceTracking(() => [api]);
    const newer = initPresenceTracking(() => [api]);
    stop = () => {
      older.stop();
      newer.stop();
    };

    older.stop();
    newer.sync();
    await settle();

    await setPresenceStatus(scope, PresenceStatus.AWAY);
    expect(api.setPreference).toHaveBeenLastCalledWith(PresenceStatus.AWAY, expect.any(String));
  });
});
