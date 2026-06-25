import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import ProfilePage from './+page.svelte';
import { q } from '$lib/test-utils';

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  mutation: vi.fn(),
  currentUser: {
    user: {
      id: 'user-1',
      login: 'alice',
      displayName: 'Alice',
      avatarUrl: null
    },
    loading: false
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      currentUser: mocks.currentUser
    })
  }
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: mocks.query,
      mutation: mocks.mutation,
      subscription: vi.fn()
    }
  })
}));

function settle() {
  return Promise.resolve()
    .then(() => Promise.resolve())
    .then(() => flushSync());
}

function setInputValue(input: HTMLInputElement, value: string) {
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
      avatarUrl: null
    };
    mocks.query.mockReset();
    mocks.query.mockReturnValue({
      toPromise: vi.fn().mockResolvedValue({
        data: { viewer: { user: { id: 'user-1', lastLoginChange: null } } },
        error: null
      })
    });
    mocks.mutation.mockReset();
    mocks.mutation.mockImplementation((_document, variables) => ({
      toPromise: vi.fn().mockResolvedValue({
        data: {
          updateProfile: {
            id: 'user-1',
            displayName: variables.input.displayName ?? mocks.currentUser.user!.displayName,
            login: variables.input.login ?? mocks.currentUser.user!.login
          }
        },
        error: null
      })
    }));
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

    await expect.element(displayNameInput).toHaveValue('Alice');
    await expect.element(usernameInput).toHaveValue('alice');
    await expect.element(saveButton).toBeDisabled();
  });

  it('submits a valid display name through the profile mutation', async () => {
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
      expect(mocks.mutation).toHaveBeenCalledWith(expect.anything(), {
        input: {
          userId: 'user-1',
          displayName: 'Ada Lovelace',
          login: null
        }
      });
    });
    await expect.element(q(container, 'form')).toHaveTextContent('Profile updated successfully');
    await expect.element(displayNameInput).toHaveValue('Ada Lovelace');
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
    expect(mocks.mutation).not.toHaveBeenCalled();
  });
});
