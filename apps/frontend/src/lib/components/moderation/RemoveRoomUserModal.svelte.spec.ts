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

    await expect.element(page.getByTestId('remove-room-user-card')).toHaveTextContent('Target');
    await expect.element(page.getByTestId('remove-room-user-card')).toHaveTextContent('@target');
    await expect
      .element(page.getByRole('combobox', { name: 'Suspension period' }))
      .toHaveValue('none');
    await page.getByRole('textbox', { name: 'Reason' }).fill('reset participation');
    await page.getByRole('button', { name: 'Remove from room' }).click();

    expect(onconfirm).toHaveBeenCalledWith('reset participation', { kind: 'none' });
  });

  it('offers timed and indefinite suspensions in one dropdown', async () => {
    const onconfirm = vi.fn();
    render(RemoveRoomUserModal, { props: { user, onconfirm } });

    await page.getByRole('textbox', { name: 'Reason' }).fill('cooldown');
    await page.getByRole('combobox', { name: 'Suspension period' }).selectOptions('24h');
    await page.getByRole('button', { name: 'Remove from room' }).click();

    expect(onconfirm).toHaveBeenCalledWith('cooldown', {
      kind: 'until',
      expiresAt: expect.any(String)
    });
    expect(Date.parse(onconfirm.mock.calls[0][1].expiresAt)).toBeGreaterThan(Date.now());
  });

  it('requires a suspension for a Universal room', async () => {
    const onconfirm = vi.fn();
    render(RemoveRoomUserModal, { props: { user, isUniversal: true, onconfirm } });

    const suspension = page.getByRole('combobox', { name: 'Suspension period' });
    await expect.element(suspension).toHaveValue('indefinite');
    await expect
      .element(page.getByRole('option', { name: 'No suspension — can rejoin' }))
      .not.toBeInTheDocument();
    await page.getByRole('textbox', { name: 'Reason' }).fill('cooldown');
    await page.getByRole('button', { name: 'Remove from room' }).click();

    expect(onconfirm).toHaveBeenCalledWith('cooldown', { kind: 'indefinite' });
  });

  it('requires a future custom end time', async () => {
    const onconfirm = vi.fn();
    render(RemoveRoomUserModal, { props: { user, onconfirm } });

    await page.getByRole('textbox', { name: 'Reason' }).fill('cooldown');
    await page.getByRole('combobox', { name: 'Suspension period' }).selectOptions('custom');
    const submit = page.getByRole('button', { name: 'Remove from room' });
    const custom = page.getByRole('textbox', { name: 'Custom expiry' });
    await expect.element(submit).toBeDisabled();
    await custom.fill('2020-01-01T12:00');
    await expect.element(submit).toBeDisabled();
    await custom.fill('2099-01-01T12:00');
    await expect.element(submit).toBeEnabled();
    await submit.click();

    expect(onconfirm).toHaveBeenCalledWith('cooldown', {
      kind: 'until',
      expiresAt: expect.any(String)
    });
  });
});
