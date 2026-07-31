import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
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
      connectBaseUrl: 'https://chat.example.test/api/connect',
      bearerToken: 'token'
    },
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('$lib/api-client/serverState', () => mocks);

beforeEach(() => {
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
