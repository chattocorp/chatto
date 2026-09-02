import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import ProfilePage from './+page.svelte';
import { q } from '$lib/test-utils';

const avatarDataUrl = 'data:image/gif;base64,R0lGODlhAQABAAAAACwAAAAAAQABAAA=';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutation: vi.fn(),
  updateProfile: vi.fn(),
  uploadAvatar: vi.fn(),
  deleteAvatar: vi.fn(),
  supportsUserAvatars: true,
  currentUser: {
    user: {
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      bio: null,
      viewerCanDeleteAccount: true,
      lastLoginChange: null as string | null
    },
    loading: false
  },
  permissions: {
    canAdminManageAccounts: false
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: {
      currentUser: mocks.currentUser,
      permissions: mocks.permissions,
      serverInfo: {
        supportsFeature: (feature: string) => feature !== 'userAvatars' || mocks.supportsUserAvatars
      }
    },
    connection: {
      isConnected: true,
      showConnectionLostBanner: false,
      connectBaseUrl: '/api/connect',
      bearerToken: null,
      getAPI: (factory: (config: never) => unknown) => factory({} as never),
      client: {
        query: mocks.query,
        mutation: mocks.mutation,
        subscription: vi.fn()
      }
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/api-client/account', () => ({
  createAccountAPI: () => ({
    updateProfile: mocks.updateProfile
  })
}));

vi.mock('$lib/api-client/users', () => ({
  createUserAPI: () => ({
    uploadAvatar: mocks.uploadAvatar,
    deleteAvatar: mocks.deleteAvatar
  })
}));

function settle() {
  return Promise.resolve()
    .then(() => Promise.resolve())
    .then(() => flushSync());
}

function setInputValue(input: HTMLInputElement | HTMLTextAreaElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

describe('Profile settings page', () => {
  beforeEach(() => {
    mocks.currentUser.user = {
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null,
      bio: null,
      viewerCanDeleteAccount: true,
      lastLoginChange: null
    };
    mocks.query.mockReset();
    mocks.permissions.canAdminManageAccounts = false;
    mocks.supportsUserAvatars = true;
    mocks.mutation.mockReset();
    mocks.updateProfile.mockReset();
    mocks.updateProfile.mockImplementation((input) =>
      Promise.resolve({
        id: 'user-1',
        displayName: input.displayName ?? mocks.currentUser.user!.displayName,
        login: input.login ?? mocks.currentUser.user!.login,
        avatarUrl: mocks.currentUser.user!.avatarUrl,
        bio: input.bio ?? mocks.currentUser.user!.bio
      })
    );
    mocks.uploadAvatar.mockReset();
    mocks.uploadAvatar.mockResolvedValue({
      id: 'user-1',
      displayName: 'Alice',
      login: 'alice',
      avatarUrl: avatarDataUrl
    });
    mocks.deleteAvatar.mockReset();
  });

  it('renders the current profile and keeps Save disabled until a field changes', async () => {
    const { container } = render(ProfilePage);
    await settle();

    const displayNameInput = q(
      container,
      'input[placeholder="Enter your display name"]'
    ) as HTMLInputElement;
    const usernameInput = q(container, '[data-testid="settings-username"]') as HTMLInputElement;
    const saveButton = q(container, 'button[type="submit"]') as HTMLButtonElement;
    const uploadButton = [...container.querySelectorAll<HTMLButtonElement>('button')].find(
      (button) => button.textContent?.includes('Upload avatar')
    );

    expect(container.querySelectorAll('.panel-shell')).toHaveLength(2);
    await expect.element(displayNameInput).toHaveValue('Alice');
    await expect.element(usernameInput).toHaveValue('alice');
    await expect.element(saveButton).toBeDisabled();
    expect(uploadButton).toHaveClass('btn-action');
  });

  it('hides the avatar editor when the server does not support targeted avatars', async () => {
    mocks.supportsUserAvatars = false;
    const { container } = render(ProfilePage);
    await settle();

    expect(container.querySelector('input[type="file"]')).toBeNull();
    expect(container.querySelectorAll('.panel-shell')).toHaveLength(1);
  });

  it('submits a valid display name through the account API', async () => {
    const { container } = render(ProfilePage);
    await settle();

    const displayNameInput = q(
      container,
      'input[placeholder="Enter your display name"]'
    ) as HTMLInputElement;
    setInputValue(displayNameInput, 'Ada Lovelace');

    const saveButton = q(container, 'button[type="submit"]') as HTMLButtonElement;
    await expect.element(saveButton).toBeEnabled();
    saveButton.click();

    await vi.waitFor(() => {
      expect(mocks.updateProfile).toHaveBeenCalledWith({
        displayName: 'Ada Lovelace',
        login: undefined,
        bio: undefined
      });
    });
    await expect.element(q(container, 'form')).toHaveTextContent('Profile updated successfully');
    await expect.element(displayNameInput).toHaveValue('Ada Lovelace');
  });

  it('sends a trimmed sparse bio update', async () => {
    const { container } = render(ProfilePage);
    await settle();

    const bioInput = q(container, '[data-testid="settings-bio"]') as HTMLTextAreaElement;
    setInputValue(bioInput, '  I build analytical engines.  ');
    (q(container, 'button[type="submit"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.updateProfile).toHaveBeenCalledWith({
        displayName: undefined,
        login: undefined,
        bio: 'I build analytical engines.'
      });
    });
    await expect.element(bioInput).toHaveValue('I build analytical engines.');
  });

  it('keeps the bio draft when saving fails', async () => {
    mocks.updateProfile.mockRejectedValue(new Error('Profile changed; reload and try again.'));
    const { container } = render(ProfilePage);
    await settle();

    const bioInput = q(container, '[data-testid="settings-bio"]') as HTMLTextAreaElement;
    setInputValue(bioInput, 'Unsaved profile draft');
    (q(container, 'button[type="submit"]') as HTMLButtonElement).click();

    await expect
      .element(q(container, 'form'))
      .toHaveTextContent('Profile changed; reload and try again.');
    await expect.element(bioInput).toHaveValue('Unsaved profile draft');
  });

  it('shows client validation errors without calling the profile mutation', async () => {
    const { container } = render(ProfilePage);
    await settle();

    const displayNameInput = q(
      container,
      'input[placeholder="Enter your display name"]'
    ) as HTMLInputElement;
    setInputValue(displayNameInput, 'John  Doe');

    (q(container, 'button[type="submit"]') as HTMLButtonElement).click();

    await expect.element(q(container, 'form')).toHaveTextContent('consecutive spaces');
    expect(mocks.updateProfile).not.toHaveBeenCalled();
  });

  it('confirms a username change before updating the profile', async () => {
    const { container } = render(ProfilePage);
    await settle();

    const usernameInput = q(container, '[data-testid="settings-username"]') as HTMLInputElement;
    setInputValue(usernameInput, 'alice2');
    (q(container, 'button[type="submit"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(container.textContent).toContain(
        'Are you sure you want to change your username to @alice2?'
      );
    });
    expect(mocks.updateProfile).not.toHaveBeenCalled();

    const confirmButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>('dialog button')
    ).find((button) => button.textContent?.includes('Change username'));
    expect(confirmButton).toBeDefined();
    confirmButton?.click();

    await vi.waitFor(() => {
      expect(mocks.updateProfile).toHaveBeenCalledWith({
        displayName: undefined,
        login: 'alice2',
        bio: undefined
      });
    });
  });

  it('keeps the username cooldown for a regular user', async () => {
    mocks.currentUser.user.lastLoginChange = new Date().toISOString();
    const { container } = render(ProfilePage);
    await settle();

    const usernameInput = q(container, '[data-testid="settings-username"]') as HTMLInputElement;
    await expect.element(usernameInput).toBeDisabled();
    await expect.element(q(container, 'form')).toHaveTextContent(
      'You can change your username again in'
    );
  });

  it('lets an account manager bypass their own username cooldown', async () => {
    const lastLoginChange = new Date().toISOString();
    mocks.currentUser.user.lastLoginChange = lastLoginChange;
    mocks.permissions.canAdminManageAccounts = true;
    const { container } = render(ProfilePage);
    await settle();

    const usernameInput = q(container, '[data-testid="settings-username"]') as HTMLInputElement;
    await expect.element(usernameInput).toBeEnabled();
    expect(container.textContent).not.toContain('You can change your username again in');

    setInputValue(usernameInput, 'alice2');
    (q(container, 'button[type="submit"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(container.textContent).toContain(
        'Are you sure you want to change your username to @alice2?'
      );
    });
    expect(container.textContent).not.toContain(
      'You can only change your username once every 30 days.'
    );

    const confirmButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>('dialog button')
    ).find((button) => button.textContent?.includes('Change username'));
    expect(confirmButton).toBeDefined();
    confirmButton?.click();

    await vi.waitFor(() => {
      expect(mocks.updateProfile).toHaveBeenCalledWith({
        displayName: undefined,
        login: 'alice2',
        bio: undefined
      });
    });
    expect(mocks.currentUser.user.lastLoginChange).toBe(lastLoginChange);
  });

  it('uploads an avatar through the targeted user API', async () => {
    const { container } = render(ProfilePage);
    await settle();

    const input = q(container, 'input[type="file"]') as HTMLInputElement;
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'avatar.png', {
      type: 'image/png'
    });
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [file]
    });
    input.dispatchEvent(new Event('change', { bubbles: true }));

    await vi.waitFor(() => {
      expect(mocks.uploadAvatar).toHaveBeenCalledWith('user-1', file);
    });
    expect(mocks.currentUser.user?.avatarUrl).toBe(avatarDataUrl);
    await vi.waitFor(() => {
      const img = container.querySelector('img') as HTMLImageElement | null;
      expect(img?.src).toBe(avatarDataUrl);
    });
  });
});
