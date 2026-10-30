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
  it('shows the server profile and renders welcome text as Markdown', async () => {
    const screen = render(ServerInformation, {
      name: 'Makers community',
      host: 'chat.example.test',
      version: '0.5.0-beta.16',
      compatibility: { status: 'supported', reason: 'version-confirmed' },
      features: [],
      iconUrl: '/icons/icon-192.png',
      bannerUrl: '/icons/icon-512.png',
      description: 'Share your projects.',
      welcomeMessage: 'Welcome, **makers**!'
    });
    await expect
      .element(screen.getByRole('heading', { name: 'Makers community', level: 2 }))
      .toBeVisible();
    await expect.element(screen.getByText('Share your projects.')).toBeVisible();
    await vi.waitFor(() => {
      expect(screen.container.querySelector('strong')?.textContent).toBe('makers');
    });
    expect(screen.container.querySelectorAll('img')).toHaveLength(2);
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
    expect(screen.container.querySelector('#independentSlowMode')).not.toBeNull();
    await expect.element(screen.getByText('Requires 0.5.0-beta.18 or newer')).toBeVisible();
    expect(screen.container.textContent?.match(/ask the server administrator/g)).toHaveLength(1);
  });
  it('shows a supported state without upgrade advice', async () => {
    const screen = show('0.5.0-beta.18');
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

  it('withholds the success notice when the catalog fails and offers retry', async () => {
    const retry = vi.fn();
    const screen = render(ServerInformation, {
      name: 'Example',
      host: 'chat.example.test',
      version: '0.6.0',
      compatibility: evaluateServerCompatibility({ serverVersion: '0.6.0' }),
      features: [],
      frontendFeaturesError: 'Catalog unavailable',
      onRetryFrontendFeatures: retry
    });
    await expect.element(screen.getByText('Catalog unavailable')).toBeVisible();
    expect(screen.container.textContent).not.toContain('No known compatibility issues');
    await screen.getByRole('button', { name: 'Try Again' }).click();
    expect(retry).toHaveBeenCalledOnce();
  });

  it('shows unknown future features as plain text and clears guidance after an app update', async () => {
    const props = {
      name: 'Example community',
      host: 'chat.example.test',
      version: '0.6.0',
      compatibility: evaluateServerCompatibility({ serverVersion: '0.6.0' }),
      features: [],
      frontendVersion: '0.5.0',
      frontendFeatures: [
        {
          id: 'future-feature',
          name: '<img src=x onerror=alert(1)>',
          description: '<script>alert(1)</script>',
          introducedVersion: '0.6.0'
        }
      ]
    };
    const screen = render(ServerInformation, props);
    await expect
      .element(screen.getByText('Your app does not support all features on this server'))
      .toBeVisible();
    await expect.element(screen.getByText('Requires app version 0.6.0 or newer')).toBeVisible();
    const report = screen.container.querySelector('[data-testid="feature-limitations"]');
    await expect
      .element(screen.getByRole('heading', { name: 'Affected features (1)' }))
      .toBeVisible();
    expect(report?.textContent).toContain('<img src=x onerror=alert(1)>');
    expect(report?.textContent).toContain('<script>alert(1)</script>');
    expect(report?.querySelector('img, script')).toBeNull();
    expect(screen.container.textContent).not.toContain('No known compatibility issues');
    await screen.rerender({ ...props, frontendVersion: '0.6.0', frontendFeatures: [] });
    await expect.element(screen.getByText('No known compatibility issues')).toBeVisible();
    expect(screen.container.querySelector('[data-testid="feature-limitations"]')).toBeNull();
  });

  it('uses one feature list for both directions and hides it while loading or unreachable', async () => {
    const props = {
      name: 'Example',
      host: 'chat.example.test',
      version: '0.5.0-beta.11',
      compatibility: evaluateServerCompatibility({ serverVersion: '0.5.0-beta.11' }),
      features: limitedServerFeatures('0.5.0-beta.11'),
      frontendVersion: '0.5.0',
      frontendFeatures: [
        {
          id: 'future',
          name: 'Future feature',
          description: 'New functionality.',
          introducedVersion: '0.6.0'
        }
      ]
    };
    const screen = render(ServerInformation, props);
    await expect
      .element(screen.getByRole('heading', { name: 'Affected features (3)' }))
      .toBeVisible();
    const report = screen.container.querySelector('[data-testid="feature-limitations"]');
    expect(report?.querySelectorAll('section')).toHaveLength(3);
    expect(report?.querySelector('#threadViews')).not.toBeNull();
    expect(report?.querySelector('#frontend-future')).not.toBeNull();
    for (const row of report?.querySelectorAll('section') ?? []) {
      expect(row.querySelector('h4')?.id).toBe(row.getAttribute('aria-labelledby'));
      expect(row.querySelectorAll('p')).toHaveLength(2);
    }
    await screen.rerender({ ...props, loading: true });
    expect(screen.container.querySelector('[data-testid="feature-limitations"]')).toBeNull();
    await screen.rerender({
      ...props,
      loading: false,
      compatibility: { status: 'unreachable', reason: 'unreachable' }
    });
    expect(screen.container.querySelector('[data-testid="feature-limitations"]')).toBeNull();
    expect(screen.container.textContent).not.toContain('Your app does not support');
  });
});
