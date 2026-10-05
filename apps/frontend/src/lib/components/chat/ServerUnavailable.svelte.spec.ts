import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { MINIMUM_SUPPORTED_SERVER_VERSION } from '@chatto/client/server/compatibility';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({ pushState: vi.fn() }));

vi.mock('$app/navigation', () => ({ pushState: mocks.pushState }));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

import ServerUnavailable from './ServerUnavailable.svelte';

const registration = {
  name: 'Saved Server',
  url: 'https://chat.example.test:8443',
  iconUrl: null
};

function serverInfo(overrides: { name?: string; version?: string } = {}) {
  return { name: 'Old Server', version: '0.5.0-beta.7', iconUrl: null, ...overrides };
}

describe('ServerUnavailable', () => {
  it('explains a server that is too old, with both versions', async () => {
    createTestServerScope({ serverInfo: serverInfo() });
    render(ServerUnavailable, {
      props: { reason: 'server-too-old', registration, onretry: vi.fn() }
    });

    await expect
      .element(page.getByRole('heading', { name: 'Server upgrade required' }))
      .toBeVisible();
    await expect.element(page.getByText('Old Server')).toBeVisible();
    await expect.element(page.getByText('chat.example.test:8443')).toBeVisible();
    await expect.element(page.getByText(/Ask the server administrator/)).toBeVisible();
    await expect
      .element(page.getByTestId('server-unavailable-version'))
      .toHaveTextContent('0.5.0-beta.7');
    await expect
      .element(page.getByTestId('server-unavailable-required-version'))
      .toHaveTextContent(`${MINIMUM_SUPPORTED_SERVER_VERSION} or newer`);
  });

  it('omits an empty server version when the version is unknown', async () => {
    createTestServerScope({ serverInfo: serverInfo({ version: '' }) });
    render(ServerUnavailable, {
      props: { reason: 'server-version-unknown', registration, onretry: vi.fn() }
    });

    await expect
      .element(page.getByRole('heading', { name: 'Unknown server version' }))
      .toBeVisible();
    await expect.element(page.getByTestId('server-unavailable-version')).not.toBeInTheDocument();
    await expect.element(page.getByTestId('server-unavailable-required-version')).toBeVisible();
  });

  it('names an unreachable server from its registration and hides version facts', async () => {
    // Failed discovery keeps the default name.
    createTestServerScope({ serverInfo: serverInfo({ name: 'Chatto', version: '' }) });
    render(ServerUnavailable, {
      props: { reason: 'unreachable', registration, onretry: vi.fn() }
    });

    await expect.element(page.getByRole('heading', { name: 'Server unreachable' })).toBeVisible();
    await expect.element(page.getByText('Saved Server')).toBeVisible();
    await expect
      .element(page.getByTestId('server-unavailable-required-version'))
      .not.toBeInTheDocument();
  });

  it('runs discovery again and shows progress until it settles', async () => {
    createTestServerScope({ serverInfo: serverInfo() });
    let settle!: () => void;
    const onretry = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          settle = resolve;
        })
    );
    render(ServerUnavailable, { props: { reason: 'server-too-old', registration, onretry } });

    await page.getByRole('button', { name: 'Check Again' }).click();
    // The loading state replaces the label, so find the only button by role.
    const button = page.getByRole('button');
    expect(onretry).toHaveBeenCalledTimes(1);
    await expect.element(button).toBeDisabled();

    settle();
    await expect.element(button).toBeEnabled();
  });

  it('offers to remove a server that the caller allows to remove', async () => {
    createTestServerScope({ serverId: 'remote', serverInfo: serverInfo() });
    render(ServerUnavailable, {
      props: { reason: 'server-too-old', registration, removable: true, onretry: vi.fn() }
    });

    await page.getByRole('button', { name: 'Remove server' }).click();

    expect(mocks.pushState).toHaveBeenCalledWith('', {
      modal: { type: 'removeServer', serverId: 'remote', spaceName: 'Old Server' }
    });
  });

  it('does not offer to remove the origin server', async () => {
    createTestServerScope({ serverInfo: serverInfo() });
    render(ServerUnavailable, {
      props: { reason: 'server-too-old', registration, onretry: vi.fn() }
    });

    await expect.element(page.getByRole('button', { name: 'Check Again' })).toBeVisible();
    await expect
      .element(page.getByRole('button', { name: 'Remove server' }))
      .not.toBeInTheDocument();
  });
});
