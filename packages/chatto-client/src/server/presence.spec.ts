import { describe, expect, it } from 'vitest';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { ServerPresence } from './presence.js';

describe('ServerPresence', () => {
  it('stores live changes and ignores unspecified statuses', () => {
    const presence = new ServerPresence();

    presence.set('U1', PresenceStatus.AWAY);
    presence.set('U1', PresenceStatus.UNSPECIFIED);

    expect(presence.get('U1')).toBe(PresenceStatus.AWAY);
    expect(presence.get('U2')).toBeUndefined();
  });

  it('merges partial snapshots and replaces on a complete snapshot', () => {
    const presence = new ServerPresence();
    presence.set('U1', PresenceStatus.ONLINE);

    presence.applySnapshot(
      [
        ['U1', PresenceStatus.UNSPECIFIED],
        ['U2', PresenceStatus.AWAY]
      ],
      false
    );

    // A snapshot without presence keeps the known status.
    expect(presence.get('U1')).toBe(PresenceStatus.ONLINE);
    expect(presence.get('U2')).toBe(PresenceStatus.AWAY);

    presence.applySnapshot([['U2', PresenceStatus.OFFLINE]], true);

    expect(presence.get('U1')).toBeUndefined();
    expect(presence.get('U2')).toBe(PresenceStatus.OFFLINE);
  });

  it('lets a change during a preview read win over the preview', () => {
    const presence = new ServerPresence();
    const readVersion = presence.version;

    presence.set('U1', PresenceStatus.OFFLINE);
    presence.applyRead('U1', PresenceStatus.ONLINE, readVersion);
    presence.applyRead('U2', PresenceStatus.ONLINE, readVersion);

    expect(presence.get('U1')).toBe(PresenceStatus.OFFLINE);
    expect(presence.get('U2')).toBe(PresenceStatus.ONLINE);
  });

  it('fences a snapshot that this client read against newer changes', () => {
    const presence = new ServerPresence();
    const readVersion = presence.version;
    presence.set('U1', PresenceStatus.OFFLINE);

    presence.applySnapshot(
      [
        ['U1', PresenceStatus.ONLINE],
        ['U2', PresenceStatus.AWAY]
      ],
      false,
      readVersion
    );

    expect(presence.get('U1')).toBe(PresenceStatus.OFFLINE);
    expect(presence.get('U2')).toBe(PresenceStatus.AWAY);
  });

  it('forgets everything on clear and reports the change', () => {
    const presence = new ServerPresence();
    presence.set('U1', PresenceStatus.ONLINE);
    const version = presence.version;

    presence.clear();

    expect(presence.get('U1')).toBeUndefined();
    expect(presence.version).toBeGreaterThan(version);
  });
});
