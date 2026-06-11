import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import type { Client } from '@urql/svelte';
import { q } from '$lib/test-utils';
import { AdminRoomLayoutStore, type AdminRoomInfo } from '$lib/state/server/adminRoomLayout.svelte';
import AdminRoomLayoutEditor from './AdminRoomLayoutEditor.svelte';

vi.mock('$app/navigation', () => ({
  afterNavigate: vi.fn(),
  beforeNavigate: vi.fn(),
  disableScrollHandling: vi.fn(),
  goto: vi.fn(),
  invalidate: vi.fn(),
  invalidateAll: vi.fn(),
  onNavigate: vi.fn(),
  preloadCode: vi.fn(),
  preloadData: vi.fn(),
  pushState: vi.fn(),
  replaceState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  assets: '',
  base: '',
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[groupId]', params?.groupId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('svelte-dnd-action', () => ({
  dndzone: () => ({
    update: vi.fn(),
    destroy: vi.fn()
  })
}));

function room(id: string, overrides: Partial<AdminRoomInfo> = {}): AdminRoomInfo {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    archived: overrides.archived ?? false
  };
}

function makeLayout(): AdminRoomLayoutStore {
  const client = {
    query: vi.fn(),
    mutation: vi.fn(),
    subscription: vi.fn()
  } as unknown as Client;
  return new AdminRoomLayoutStore(client);
}

function renderEditor(layout: AdminRoomLayoutStore) {
  return render(AdminRoomLayoutEditor, {
    props: { layout, serverSegment: '-' }
  });
}

function buttonByText(container: Element, text: string): HTMLButtonElement {
  const button = [...container.querySelectorAll('button')].find((b) =>
    b.textContent?.includes(text)
  );
  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
}

function fill(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

describe('AdminRoomLayoutEditor', () => {
  it('renders loading, error, empty, and populated states from the layout store', async () => {
    const loading = makeLayout();
    loading.isRefreshing = true;
    const loadingRender = renderEditor(loading);
    await expect.element(q(loadingRender.container, 'div')).toHaveTextContent('Loading rooms...');

    const error = makeLayout();
    error.error = 'Server not found';
    const errorRender = renderEditor(error);
    expect(errorRender.container.textContent).toContain('Server not found');

    const empty = makeLayout();
    empty.initialized = true;
    const emptyRender = renderEditor(empty);
    expect(emptyRender.container.textContent).toContain('No room groups yet');

    const populated = makeLayout();
    populated.initialized = true;
    populated.groups = [
      {
        id: 'g1',
        name: 'Lobby',
        rooms: [room('r1', { name: 'general', description: 'Public room' })]
      }
    ];
    const populatedRender = renderEditor(populated);
    expect(populatedRender.container.textContent).toContain('Lobby');
    expect(populatedRender.container.textContent).toContain('general');
    expect(populatedRender.container.textContent).toContain('Public room');
  });

  it('opens the create-group dialog and delegates submission to the layout store', async () => {
    const layout = makeLayout();
    layout.initialized = true;
    layout.groups = [{ id: 'g1', name: 'Lobby', rooms: [] }];
    const createGroup = vi.spyOn(layout, 'createGroup').mockResolvedValue({
      ok: true,
      group: { id: 'g2', name: 'Projects', rooms: [] }
    });
    const { container } = renderEditor(layout);

    buttonByText(container, 'New Group').click();
    flushSync();
    fill(q(container, '#new-group-name') as HTMLInputElement, 'Projects');
    buttonByText(container, 'Create Group').click();

    await vi.waitFor(() => {
      expect(createGroup).toHaveBeenCalledWith('Projects');
    });
  });

  it('keeps Save disabled and shows validation when a room name has leading whitespace', async () => {
    const layout = makeLayout();
    layout.initialized = true;
    layout.groups = [{ id: 'g1', name: 'Lobby', rooms: [room('r1', { name: 'general' })] }];
    const updateRoom = vi.spyOn(layout, 'updateRoom').mockResolvedValue({ ok: true });
    const { container } = renderEditor(layout);

    const edit = container.querySelector('[title="Edit room"]');
    if (!(edit instanceof HTMLButtonElement)) throw new Error('edit button not found');
    edit.click();
    flushSync();

    const input = q(container, '#edit-room-name') as HTMLInputElement;
    fill(input, ' bad-name');

    expect(container.textContent).toContain('Room name cannot have leading or trailing whitespace');
    const save = buttonByText(container, 'Save Changes');
    expect(save.disabled).toBe(true);
    save.click();
    await Promise.resolve();
    expect(updateRoom).not.toHaveBeenCalled();
  });
});
