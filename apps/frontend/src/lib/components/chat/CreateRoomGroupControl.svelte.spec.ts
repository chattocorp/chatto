import { beforeEach, describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    createRoomGroup: vi.fn().mockResolvedValue(null),
    isCurrent: vi.fn().mockReturnValue(true)
  }
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    connection: {
      getAPI: () => ({ createRoomGroup: mocks.createRoomGroup })
    },
    isCurrent: mocks.isCurrent
  })
}));

import CreateRoomGroupControl from './CreateRoomGroupControl.svelte';

beforeEach(() => {
  vi.clearAllMocks();
  mocks.createRoomGroup.mockResolvedValue(null);
  mocks.isCurrent.mockReturnValue(true);
});

describe('CreateRoomGroupControl', () => {
  it('creates a trimmed room group from the pinned inline form', async () => {
    const { container, getByRole } = render(CreateRoomGroupControl);

    await userEvent.click(getByRole('button', { name: 'New Group' }));
    const input = container.querySelector<HTMLInputElement>('#sidebar-new-room-group');
    expect(input).not.toBeNull();
    await userEvent.fill(input!, '  Projects  ');
    await userEvent.click(getByRole('button', { name: 'Create Group' }));

    await vi.waitFor(() => {
      expect(mocks.createRoomGroup).toHaveBeenCalledWith({ name: 'Projects' });
    });
    await expect.element(getByRole('button', { name: 'New Group' })).toBeInTheDocument();
  });

  it('closes the lightweight form with Escape without creating a group', async () => {
    const { container, getByRole } = render(CreateRoomGroupControl);

    await userEvent.click(getByRole('button', { name: 'New Group' }));
    const input = container.querySelector<HTMLInputElement>('#sidebar-new-room-group');
    expect(input).not.toBeNull();
    await userEvent.fill(input!, 'Projects');
    await userEvent.keyboard('{Escape}');

    await expect.element(getByRole('button', { name: 'New Group' })).toBeInTheDocument();
    expect(mocks.createRoomGroup).not.toHaveBeenCalled();
  });
});
