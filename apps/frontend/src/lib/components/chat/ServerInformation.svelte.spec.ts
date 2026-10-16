import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import {
  evaluateServerCompatibility,
  limitedServerFeatures
} from '@chatto/client/server/compatibility';
import ServerInformation from './ServerInformation.svelte';
import { toast } from '$lib/ui/toast';

describe('ServerInformation', () => {
  afterEach(() => vi.restoreAllMocks());
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });
  function show(version: string) {
    return render(ServerInformation, {
      name: 'Example community',
      host: 'chat.example.test:8443',
      version,
      compatibility: evaluateServerCompatibility({ serverVersion: version }),
      features: limitedServerFeatures(version)
    });
  }
  it('copies the hostname with its non-default port', async () => {
    const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
    const success = vi.spyOn(toast, 'success').mockReturnValue('toast-id');
    const screen = show('0.5.0-beta.16');
    await screen.getByRole('button', { name: 'Copy server hostname' }).click();
    expect(writeText).toHaveBeenCalledWith('chat.example.test:8443');
    expect(success).toHaveBeenCalled();
  });
  it('explains clipboard failures', async () => {
    vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('denied'));
    const error = vi.spyOn(toast, 'error').mockReturnValue('toast-id');
    const screen = show('0.5.0-beta.16');
    await screen.getByRole('button', { name: 'Copy server hostname' }).click();
    expect(error).toHaveBeenCalled();
  });
  it('shows one upgrade recommendation and a linkable feature explanation', async () => {
    const screen = show('0.5.0-beta.11');
    await expect.element(screen.getByText('Version 0.5.0-beta.11')).toBeVisible();
    await expect.element(screen.getByText('Requires 0.5.0-beta.16 or newer')).toBeVisible();
    expect(screen.container.querySelector('#threadViews')).not.toBeNull();
    expect(screen.container.textContent?.match(/ask the server administrator/g)).toHaveLength(1);
  });
  it('shows a supported state without upgrade advice', async () => {
    const screen = show('0.5.0-beta.16');
    await expect.element(screen.getByText('No known compatibility issues')).toBeVisible();
    expect(screen.container.querySelector('#threadViews')).toBeNull();
  });
  it('does not treat an unknown version as supported', async () => {
    const screen = show('dev');
    await expect
      .element(screen.getByText('This app cannot determine compatibility from the server version.'))
      .toBeVisible();
    expect(screen.container.textContent).not.toContain('No known compatibility issues');
  });
});
