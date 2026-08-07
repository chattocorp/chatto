import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { adminQueryKeys } from '$lib/query/admin';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import { queryClient } from '$lib/query/client';
import ServerSettings from './ServerSettings.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    getAuthenticatedServerState: vi.fn(),
    updateServerConfig: vi.fn(),
    uploadServerLogo: vi.fn(),
    deleteServerLogo: vi.fn(),
    uploadServerBanner: vi.fn(),
    deleteServerBanner: vi.fn(),
    goto: vi.fn(),
    scopeCurrent: true
  }
}));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: mocks.goto
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {},
    connection: {
      serverId: 'origin',
      queryScope: 'server-settings-test',
      apiConfig: {
        baseUrl: 'https://chat.example.test/api/connect',
        bearerToken: 'token'
      },
      connectBaseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    },
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('$lib/api-client/serverState', () => mocks);

beforeEach(() => {
  queryClient.clear();
  vi.clearAllMocks();
  mocks.scopeCurrent = true;
  mocks.getAuthenticatedServerState.mockResolvedValue({
    name: 'Example server',
    description: 'Original description',
    motd: '',
    welcomeMessage: '',
    logoUrl: null,
    bannerUrl: null,
    viewerCanManageServer: true
  });
  mocks.updateServerConfig.mockResolvedValue({
    name: 'Example server',
    description: 'Saved description',
    motd: '',
    welcomeMessage: ''
  });
  mocks.uploadServerLogo.mockResolvedValue({
    logoUrl: 'https://cdn.example.test/logo.webp'
  });
  mocks.deleteServerLogo.mockResolvedValue({ logoUrl: null });
  mocks.uploadServerBanner.mockResolvedValue({
    bannerUrl: 'https://cdn.example.test/banner.webp'
  });
  mocks.deleteServerBanner.mockResolvedValue({ bannerUrl: null });
});

async function renderSettings() {
  const result = render(ServerSettings);
  await vi.waitFor(() => {
    expect(result.container.querySelector('#description')).not.toBeNull();
  });
  return result;
}

function inputDescription(textarea: HTMLTextAreaElement, value: string) {
  textarea.select();
  const beforeInput = new InputEvent('beforeinput', {
    bubbles: true,
    cancelable: true,
    data: value,
    inputType: 'insertText'
  });
  textarea.dispatchEvent(beforeInput);
  if (beforeInput.defaultPrevented) return;

  textarea.setRangeText(value, textarea.selectionStart, textarea.selectionEnd, 'end');
  textarea.dispatchEvent(new Event('input', { bubbles: true }));
}

describe('ServerSettings', () => {
  it('passes query cancellation through to the snapshot API', async () => {
    await renderSettings();

    expect(mocks.getAuthenticatedServerState).toHaveBeenCalledWith(
      {
        baseUrl: 'https://chat.example.test/api/connect',
        bearerToken: 'token'
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
  });

  it('revalidates a cached settings snapshot when the screen remounts', async () => {
    const first = await renderSettings();
    first.unmount();
    await renderSettings();

    expect(mocks.getAuthenticatedServerState).toHaveBeenCalledTimes(2);
  });

  it('communicates and enforces the 500-byte description limit', async () => {
    const { container } = await renderSettings();
    const textarea = container.querySelector<HTMLTextAreaElement>('#description')!;

    expect(textarea.maxLength).toBe(500);
    expect(container.textContent).toContain('Maximum 500 bytes');

    inputDescription(textarea, 'a'.repeat(501));
    expect(textarea.value).toBe('Original description');
  });

  it('enforces the description limit using UTF-8 bytes', async () => {
    const { container } = await renderSettings();
    const textarea = container.querySelector<HTMLTextAreaElement>('#description')!;

    inputDescription(textarea, '💬'.repeat(125));
    expect(textarea.value).toBe('💬'.repeat(125));

    inputDescription(textarea, '💬'.repeat(126));

    expect(textarea.value).toBe('💬'.repeat(125));
  });

  it('keeps the draft visible when the server rejects a save', async () => {
    mocks.updateServerConfig.mockRejectedValue(new Error('Server rejected the update'));
    const { container } = await renderSettings();
    const textarea = container.querySelector<HTMLTextAreaElement>('#description')!;

    inputDescription(textarea, 'Unsaved draft');
    container.querySelector<HTMLFormElement>('form')!.requestSubmit();

    await vi.waitFor(() => {
      expect(container.textContent).toContain('Server rejected the update');
    });
    expect(container.querySelector<HTMLTextAreaElement>('#description')?.value).toBe(
      'Unsaved draft'
    );
    expect(container.querySelector('form')).not.toBeNull();
  });

  it('replaces the exact cached snapshot after a successful save', async () => {
    const queryKey = adminQueryKeys.serverSettings('origin', {
      queryScope: 'server-settings-test'
    });
    const { container } = await renderSettings();
    inputDescription(
      container.querySelector<HTMLTextAreaElement>('#description')!,
      'Saved description'
    );
    container.querySelector<HTMLFormElement>('form')!.requestSubmit();

    await vi.waitFor(() =>
      expect(queryClient.getQueryData<{ description: string }>(queryKey)?.description).toBe(
        'Saved description'
      )
    );
    expect(queryClient.getQueryData<{ logoUrl: string | null }>(queryKey)?.logoUrl).toBeNull();
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(
      true
    );
  });

  it('adopts a trimmed canonical description after saving', async () => {
    const { container } = await renderSettings();
    const textarea = container.querySelector<HTMLTextAreaElement>('#description')!;
    inputDescription(textarea, '  Saved description  ');
    container.querySelector<HTMLFormElement>('form')!.requestSubmit();

    await vi.waitFor(() => expect(textarea.value).toBe('Saved description'));
    expect(container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(
      true
    );
  });

  it('updates the cached logo after an upload mutation', async () => {
    const queryKey = adminQueryKeys.serverSettings('origin', {
      queryScope: 'server-settings-test'
    });
    const { container } = await renderSettings();
    const fileInput = container.querySelector<HTMLInputElement>('input[type="file"]')!;
    const transfer = new DataTransfer();
    transfer.items.add(new File(['image'], 'logo.png', { type: 'image/png' }));
    fileInput.files = transfer.files;
    fileInput.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => expect(mocks.uploadServerLogo).toHaveBeenCalledOnce());
    await vi.waitFor(() =>
      expect(queryClient.getQueryData<{ logoUrl: string | null }>(queryKey)?.logoUrl).toBe(
        'https://cdn.example.test/logo.webp'
      )
    );
  });

  it('adopts refreshed pristine fields without replacing a dirty draft', async () => {
    const { container } = await renderSettings();
    const textarea = container.querySelector<HTMLTextAreaElement>('#description')!;
    inputDescription(textarea, 'Unsaved draft');

    queryClient.setQueryData(
      adminQueryKeys.serverSettings('origin', { queryScope: 'server-settings-test' }),
      {
        name: 'Renamed elsewhere',
        description: 'Remote description',
        motd: '',
        welcomeMessage: '',
        logoUrl: null,
        bannerUrl: null,
        viewerCanManageServer: true
      }
    );
    flushSync();

    expect(container.querySelector<HTMLInputElement>('#name')?.value).toBe('Renamed elsewhere');
    expect(textarea.value).toBe('Unsaved draft');
  });

  it('does not restore settings after the admin cache is purged', async () => {
    let resolveSave!: (profile: {
      name: string;
      description: string;
      motd: string;
      welcomeMessage: string;
    }) => void;
    mocks.updateServerConfig.mockReturnValue(
      new Promise((resolve) => {
        resolveSave = resolve;
      })
    );
    const view = await renderSettings();
    const textarea = view.container.querySelector<HTMLTextAreaElement>('#description')!;
    inputDescription(textarea, 'Private draft');
    view.container.querySelector<HTMLFormElement>('form')!.requestSubmit();
    await vi.waitFor(() => expect(mocks.updateServerConfig).toHaveBeenCalledOnce());

    removeRegisteredAdminQueries('origin');
    view.unmount();
    resolveSave({
      name: 'Example server',
      description: 'Private draft',
      motd: '',
      welcomeMessage: ''
    });
    await Promise.resolve();

    expect(
      queryClient.getQueryData(
        adminQueryKeys.serverSettings('origin', { queryScope: 'server-settings-test' })
      )
    ).toBeUndefined();
  });

  it('does not allow an overlapping save after an admin cache reset', async () => {
    let resolveSave!: (profile: {
      name: string;
      description: string;
      motd: string;
      welcomeMessage: string;
    }) => void;
    mocks.updateServerConfig.mockReturnValue(
      new Promise((resolve) => {
        resolveSave = resolve;
      })
    );
    const view = await renderSettings();
    const textarea = view.container.querySelector<HTMLTextAreaElement>('#description')!;
    inputDescription(textarea, 'First save');
    view.container.querySelector<HTMLFormElement>('form')!.requestSubmit();
    await vi.waitFor(() => expect(mocks.updateServerConfig).toHaveBeenCalledOnce());

    removeRegisteredAdminQueries('origin');
    await vi.waitFor(() => expect(mocks.getAuthenticatedServerState).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(view.container.querySelector('form')).not.toBeNull());
    inputDescription(
      view.container.querySelector<HTMLTextAreaElement>('#description')!,
      'Second save'
    );
    view.container.querySelector<HTMLFormElement>('form')!.requestSubmit();

    expect(mocks.updateServerConfig).toHaveBeenCalledOnce();
    expect(view.container.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(
      true
    );
    resolveSave({
      name: 'Example server',
      description: 'First save',
      motd: '',
      welcomeMessage: ''
    });
    await Promise.resolve();
    view.unmount();
  });

  it('ignores a denied response after its server scope is replaced', async () => {
    let resolveState!: (state: { viewerCanManageServer: boolean; name: string }) => void;
    mocks.getAuthenticatedServerState.mockReturnValue(
      new Promise((resolve) => {
        resolveState = resolve;
      })
    );

    render(ServerSettings);
    await vi.waitFor(() => expect(mocks.getAuthenticatedServerState).toHaveBeenCalledOnce());

    mocks.scopeCurrent = false;
    resolveState({ viewerCanManageServer: false, name: 'Old server' });
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
