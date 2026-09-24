import { describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import RemoveRoomUserModal from './RemoveRoomUserModal.svelte';

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_id: string, fallback: string) => fallback,
  getLiveLogin: (_id: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_id: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_id: string, fallback: unknown) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({ getPresenceCache: () => null }));

const user = {
  id: 'target-1',
  login: 'target',
  displayName: 'Target',
  presenceStatus: PresenceStatus.OFFLINE
};

describe('RemoveRoomUserModal', () => {
  it('removes without suspension by default', async () => {
    const onconfirm = vi.fn();
    render(RemoveRoomUserModal, { props: { user, onconfirm } });

    await expect.element(page.getByRole('radio', { name: 'No suspension — can rejoin' })).toBeChecked();
    await page.getByRole('textbox', { name: 'Reason' }).fill('reset participation');
    await page.getByRole('button', { name: 'Remove from room' }).click();

    expect(onconfirm).toHaveBeenCalledWith('reset participation', { kind: 'none' });
  });

  it('requires a suspension choice for a Universal room', async () => {
    const onconfirm = vi.fn();
    render(RemoveRoomUserModal, { props: { user, isUniversal: true, onconfirm } });

    await expect.element(page.getByRole('radio', { name: 'No suspension — can rejoin' })).not.toBeInTheDocument();
    await page.getByRole('textbox', { name: 'Reason' }).fill('cooldown');
    await page.getByRole('combobox', { name: 'Suspension period' }).selectOptions('24h');
    await page.getByRole('button', { name: 'Remove from room' }).click();

    expect(onconfirm).toHaveBeenCalledWith('cooldown', {
      kind: 'until',
      expiresAt: expect.any(String)
    });
  });
});
