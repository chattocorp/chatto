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
    deleteServerBanner: vi.fn()
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    connectBaseUrl: 'https://chat.example.test/api/connect',
    bearerToken: 'token'
  })
}));

vi.mock('$lib/api-client/serverState', () => mocks);

beforeEach(() => {
  vi.clearAllMocks();
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
  textarea.value = value;
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

  it('rejects a middle insertion instead of deleting the existing suffix', async () => {
    const { container } = await renderSettings();
    const textarea = container.querySelector<HTMLTextAreaElement>('#description')!;
    const existingDraft = `${'💬'.repeat(123)}suffix`;
    inputDescription(textarea, existingDraft);
    textarea.setSelectionRange(10, 10);

    const beforeInput = new InputEvent('beforeinput', {
      bubbles: true,
      cancelable: true,
      data: 'inserted',
      inputType: 'insertText'
    });
    textarea.dispatchEvent(beforeInput);

    expect(beforeInput.defaultPrevented).toBe(true);
    expect(textarea.value).toBe(existingDraft);
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
});
