import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { PresenceStatus } from '$lib/gql/graphql';
import { q } from '$lib/test-utils';
import UserContextMenu from './UserContextMenu.svelte';

vi.mock('$lib/utils/isTouchDevice', () => ({
  isTouchDevice: () => false
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveLogin: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_userId: string, fallback: string) => fallback
  })
}));

const user = {
  id: 'user-1',
  login: 'alice',
  displayName: 'Alice Example',
  avatarUrl: null,
  presenceStatus: PresenceStatus.Online
};

let originalShowPopover: typeof HTMLElement.prototype.showPopover;

function renderMenu(props: Record<string, unknown> = {}) {
  return render(UserContextMenu, {
    props: {
      user,
      anchorRect: { top: 10, bottom: 30, left: 20 },
      onClose: vi.fn(),
      ...props
    }
  });
}

beforeAll(() => {
  originalShowPopover = HTMLElement.prototype.showPopover;
  HTMLElement.prototype.showPopover = function showPopover() {
    this.setAttribute('popover-open', '');
  };
});

afterAll(() => {
  HTMLElement.prototype.showPopover = originalShowPopover;
});

describe('UserContextMenu', () => {
  it('renders the user profile content', async () => {
    const { container } = renderMenu();

    await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();
    expect(container.textContent).toContain('Alice Example');
    expect(container.textContent).toContain('@alice');
  });

  it('shows Send Message only when allowed', async () => {
    const hidden = renderMenu({ canSendMessage: false });
    expect(hidden.container.textContent).not.toContain('Send Message');
    hidden.unmount();

    const visible = renderMenu({ canSendMessage: true });
    await expect
      .element(q(visible.container, 'button'))
      .toHaveTextContent('Send Message');
  });

  it('calls send and close callbacks when sending a message', () => {
    const onSendMessage = vi.fn();
    const onClose = vi.fn();
    const { container } = renderMenu({ canSendMessage: true, onSendMessage, onClose });

    (q(container, 'button') as HTMLButtonElement).click();

    expect(onSendMessage).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('closes on Escape', () => {
    const onClose = vi.fn();
    renderMenu({ onClose });

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));

    expect(onClose).toHaveBeenCalledOnce();
  });
});
