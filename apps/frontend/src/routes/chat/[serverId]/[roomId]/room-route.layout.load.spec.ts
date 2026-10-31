import { expect, it, vi } from 'vitest';

const { loadEditor, preferences } = vi.hoisted(() => ({
  loadEditor: vi.fn(),
  preferences: { composerEditor: 'markdown' }
}));
vi.mock('$lib/components/composer/loadEditor', () => ({ loadEditor }));
vi.mock('$lib/state/userPreferences.svelte', () => ({ userPreferences: preferences }));

import { load } from './+layout';

it.each(['markdown', 'visual'])(
  'starts only the selected %s editor without delaying the room',
  (kind) => {
    preferences.composerEditor = kind;
    loadEditor.mockReset().mockReturnValue(new Promise(() => {}));
    expect(load({ params: { roomId: 'room' } } as never)).toEqual({ roomId: 'room' });
    expect(loadEditor).toHaveBeenCalledExactlyOnceWith(kind);
  }
);

it('allows navigation when the optional editor import fails', async () => {
  loadEditor.mockRejectedValue(new Error('Download failed'));
  expect(load({ params: { roomId: 'room' } } as never)).toEqual({ roomId: 'room' });
  await Promise.resolve();
});
