import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import DesktopSettings from './DesktopSettings.svelte';
import { DesktopUpdates } from '$lib/desktop/updates.svelte';

describe('Desktop settings', () => {
  it('shows the installed version in development without live update controls', async () => {
    const updates = new DesktopUpdates();
    updates.loading = false;
    updates.status = {
      revision: 0,
      version: '0.5.0-beta.27',
      status: 'unavailable',
      busy: false,
      progress: null,
      availableVersion: null,
      automatic: false,
      channel: 'beta'
    };
    const view = render(DesktopSettings, { updates });
    await expect.element(view.getByText('Installed version: 0.5.0-beta.27')).toBeVisible();
    await expect
      .element(view.getByText('Automatic updates are not available in this installation.'))
      .toBeVisible();
    await expect.element(view.getByRole('button')).not.toBeInTheDocument();
    await expect.element(view.getByRole('checkbox')).not.toBeInTheDocument();
  });
  it('keeps the committed checkbox value when the host rejects saving it', async () => {
    const updates = new DesktopUpdates();
    const dispose = updates.start({
      getStatus: async () => ({
        revision: 1,
        version: '0.5.0',
        status: 'idle',
        busy: false,
        progress: null,
        availableVersion: null,
        automatic: true,
        channel: 'stable'
      }),
      onStatus: () => () => {},
      check: async () => {},
      install: async () => {},
      setPreference: async () => {
        throw new Error('save failed');
      }
    });
    await expect.poll(() => updates.loading).toBe(false);
    const view = render(DesktopSettings, { updates });
    await view.getByText('Automatically check and download', { exact: true }).click();
    await expect.element(view.getByText('The update operation failed. Try again.')).toBeVisible();
    await expect.element(view.getByRole('checkbox')).toBeChecked();
    dispose();
  });
  it('shows the unavailable state without update controls', async () => {
    const updates = new DesktopUpdates();
    updates.loading = false;
    const view = render(DesktopSettings, { updates });
    await expect
      .element(view.getByText('Automatic updates are not available in this installation.'))
      .toBeVisible();
    await expect
      .element(view.getByRole('button', { name: 'Check for updates' }))
      .not.toBeInTheDocument();
  });

  it('shows progress, locks controls, and requires an explicit restart action', async () => {
    const updates = new DesktopUpdates();
    updates.loading = false;
    updates.status = {
      revision: 1,
      version: '0.5.0',
      status: 'downloading',
      busy: true,
      progress: 42,
      availableVersion: '0.6.0',
      automatic: true,
      channel: 'stable'
    };
    const install = vi.spyOn(updates, 'install').mockResolvedValue();
    const view = render(DesktopSettings, { updates });
    await expect.element(view.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '42');
    await expect.element(view.getByRole('button', { name: 'Check for updates' })).toBeDisabled();
    updates.status = { ...updates.status, revision: 2, status: 'ready', busy: false };
    await expect
      .element(view.getByText('Restarting ends active calls and screen sharing.'))
      .toBeVisible();
    expect(install).not.toHaveBeenCalled();
    await view.getByRole('button', { name: 'Restart to update' }).click();
    expect(install).toHaveBeenCalledOnce();
    updates.status = { ...updates.status, revision: 3, status: 'error', availableVersion: null };
    // A rejected command and the host's failure snapshot must show one error.
    updates.failed = true;
    await expect.element(view.getByText('The update operation failed. Try again.')).toBeVisible();
    await expect.element(view.getByRole('button', { name: 'Check for updates' })).toBeEnabled();
  });
});
