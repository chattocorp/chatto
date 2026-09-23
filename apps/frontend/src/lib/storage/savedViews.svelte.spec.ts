import { beforeEach, describe, expect, it } from 'vitest';
import { clearAllSavedViews, clearSavedView, loadSavedView, saveView, type SavedView } from './savedViews';

function view(serverId: string, userId: string, savedAt = Date.now()): SavedView {
  return {
    version: 1,
    serverId,
    userId,
    serverName: 'Example',
    savedAt,
    rooms: [{ id: 'room', name: 'Room', messages: [{
      id: 'message', createdAt: '2026-09-23T00:00:00Z', author: 'Member', body: 'Saved text'
    }] }]
  };
}

describe('device saved views', () => {
  beforeEach(async () => {
    await clearAllSavedViews();
  });

  it('loads only the exact server and user and clears that identity', async () => {
    await saveView(view('one', 'alice'));
    await saveView(view('one', 'bob'));
    await saveView(view('two', 'alice'));
    expect((await loadSavedView('one', 'alice'))?.rooms[0].messages[0].body).toBe('Saved text');
    expect(await loadSavedView('two', 'bob')).toBeNull();
    await clearSavedView('one', 'alice');
    expect(await loadSavedView('one', 'alice')).toBeNull();
    expect(await loadSavedView('one', 'bob')).not.toBeNull();
    expect(await loadSavedView('two', 'alice')).not.toBeNull();
  });

  it('expires a view seven days after its last sync', async () => {
    await saveView(view('old', 'alice', Date.now() - 8 * 24 * 60 * 60 * 1000));
    expect(await loadSavedView('old', 'alice')).toBeNull();
  });

  it('does not replace a newer view with an older tab snapshot', async () => {
    const recent = view('one', 'alice', Date.now());
    recent.rooms[0].messages[0].body = 'Current text';
    await saveView(recent);
    await saveView(view('one', 'alice', recent.savedAt - 1_000));
    expect((await loadSavedView('one', 'alice'))?.rooms[0].messages[0].body).toBe('Current text');
  });

  it('never writes a view larger than the total device limit', async () => {
    const oversized = view('large', 'alice');
    oversized.rooms[0].messages[0].body = 'x'.repeat(11 * 1024 * 1024);
    await saveView(oversized);
    expect(await loadSavedView('large', 'alice')).toBeNull();
  });
});
