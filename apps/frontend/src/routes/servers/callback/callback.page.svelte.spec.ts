import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import CallbackPage from './+page.svelte';

const { completeServerOAuthFlowMock, gotoMock, pageState } = vi.hoisted(() => ({
  completeServerOAuthFlowMock: vi.fn(),
  gotoMock: vi.fn(),
  pageState: {
    url: 'https://app.example/servers/callback'
  }
}));

vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(pageState.url);
    }
  }
}));
vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$lib/auth/reauth', () => ({
  completeServerOAuthFlow: completeServerOAuthFlowMock
}));

describe('server OAuth callback page', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    completeServerOAuthFlowMock.mockReset();
    gotoMock.mockReset();
  });

  it('returns a popup authorization code to the waiting main client', async () => {
    pageState.url =
      'https://app.example/servers/callback?mode=popup&code=cht_ACcode&state=state-123';
    const channel = new BroadcastChannel('chatto:oauth-popup:state-123');
    const closeSpy = vi.spyOn(window, 'close').mockImplementation(() => {});
    const response = new Promise<unknown>((resolve) => {
      channel.onmessage = (event) => resolve(event.data);
    });

    render(CallbackPage);

    await expect(response).resolves.toEqual({
      type: 'chatto:oauth-popup-response',
      state: 'state-123',
      code: 'cht_ACcode',
      error: undefined,
      errorDescription: undefined
    });
    await vi.waitFor(() => expect(closeSpy).toHaveBeenCalledOnce());
    expect(completeServerOAuthFlowMock).not.toHaveBeenCalled();
    expect(gotoMock).not.toHaveBeenCalled();
    channel.close();
  });
});
