import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import AddServerDialog from './AddServerDialog.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';

const STORAGE_KEY = 'chatto:instances';

function makeProbeResponse(body: object, ok = true, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status: ok ? status : status,
    headers: { 'Content-Type': 'application/json' }
  });
}

describe('AddServerDialog', () => {
  let originalFetch: typeof fetch;

  beforeEach(() => {
    localStorage.removeItem(STORAGE_KEY);
    serverRegistry.instances = [];
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('opens with an enabled URL field and a Connect button', async () => {
    const { container } = render(AddServerDialog, {
      props: { visible: true, onclose: () => {} }
    });

    const input = container.querySelector<HTMLInputElement>('#add-server-url');
    expect(input).not.toBeNull();
    expect(input!.disabled).toBe(false);

    const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]');
    expect(submit).not.toBeNull();
    expect(submit!.textContent?.trim()).toMatch(/Connect/);
  });

  it('moves to the preview stage after a successful probe', async () => {
    globalThis.fetch = vi.fn(async () =>
      makeProbeResponse({
        name: 'Remote Chatto',
        version: '0.0.150',
        authMethods: ['password'],
        registrationOpen: true,
        authorizeUrl: '/oauth/authorize'
      })
    ) as unknown as typeof fetch;

    const { container } = render(AddServerDialog, {
      props: { visible: true, onclose: () => {} }
    });

    const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
    input.value = 'https://chat.example.com';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const form = container.querySelector('form')!;
    form.requestSubmit();

    // Wait for the probe to resolve & the preview to render.
    await vi.waitFor(() => {
      const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]');
      // Submit label is intentionally static (no server-supplied name) to
      // prevent a hostile server from impersonating trusted UI copy.
      expect(submit?.textContent ?? '').toMatch(/^\s*Sign in\s*$/);
    });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      'https://chat.example.com/api/server',
      expect.any(Object)
    );

    // The server-supplied name appears in the (visually-marked) preview
    // card, not in any action button.
    expect(container.textContent).toContain('Remote Chatto');
    expect(container.textContent).toContain('chat.example.com');
  });

  it('handles the real chat.chatto.run response shape', async () => {
    const realResponse = {
      name: 'Official Chatto Community',
      version: '0.0.150',
      authMethods: ['password', 'oidc'],
      registrationOpen: true,
      welcomeMessage: 'Welcome to the official Chatto community instance.',
      authorizeUrl: '/oauth/authorize'
    };
    globalThis.fetch = vi.fn(async () =>
      makeProbeResponse(realResponse)
    ) as unknown as typeof fetch;

    let visible = $state(true);
    const onclose = vi.fn(() => {
      visible = false;
    });
    const { container } = render(AddServerDialog, {
      props: {
        get visible() {
          return visible;
        },
        set visible(v) {
          visible = v;
        },
        onclose
      }
    });

    const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
    input.value = 'chat.chatto.run';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => {
      const submit = container.querySelector<HTMLButtonElement>('button[type="submit"]');
      expect(submit?.textContent ?? '').toMatch(/^\s*Sign in\s*$/);
    });
    // Server-supplied name renders inside the preview card body but not
    // inside any action button (anti-impersonation).
    expect(container.textContent).toContain('Official Chatto Community');

    expect(onclose).not.toHaveBeenCalled();
    expect(visible).toBe(true);
  });

  it('shows an error when the probe response is not a Chatto server', async () => {
    globalThis.fetch = vi.fn(async () =>
      makeProbeResponse({ unrelated: true })
    ) as unknown as typeof fetch;

    const { container } = render(AddServerDialog, {
      props: { visible: true, onclose: () => {} }
    });

    const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
    input.value = 'https://not-chatto.example.com';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => {
      expect(container.textContent).toContain('does not appear to be a Chatto server');
    });
  });

  it('blocks adding a server that is already connected', async () => {
    serverRegistry.instances = [
      {
        id: 'remote',
        url: 'https://chat.example.com',
        name: 'Remote',
        iconUrl: null,
        token: 'abc',
        userId: 'user-1',
        userLogin: 'someone',
        userDisplayName: null,
        userAvatarUrl: null,
        addedAt: 0
      }
    ];

    const fetchSpy = vi.fn();
    globalThis.fetch = fetchSpy as unknown as typeof fetch;

    const { container } = render(AddServerDialog, {
      props: { visible: true, onclose: () => {} }
    });

    const input = container.querySelector<HTMLInputElement>('#add-server-url')!;
    input.value = 'chat.example.com';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    container.querySelector('form')!.requestSubmit();

    await vi.waitFor(() => {
      expect(container.textContent).toContain('already connected');
    });

    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
