import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import ProfilePage from './+page.svelte';
import { q } from '$lib/test-utils';
import {
  GetProfileSettingsResponse,
  ProfileSettingsView,
  UpdateProfileResponse,
  type UpdateProfileRequest
} from '$lib/pb/chatto/api/v1/chat_pb';
import { User } from '$lib/pb/chatto/core/v1/models_pb';

const mocks = vi.hoisted(() => ({
  getProfileSettings: vi.fn(),
  updateProfile: vi.fn(),
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

vi.mock('$lib/state/server/wireEventBus.svelte', () => ({
  wireEventBusManager: {
    getClient: () => ({
      getProfileSettings: mocks.getProfileSettings,
      updateProfile: mocks.updateProfile
    })
  }
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
    mocks.getProfileSettings.mockReset();
    mocks.getProfileSettings.mockResolvedValue(
      new GetProfileSettingsResponse({
        profile: new ProfileSettingsView({
          user: new User({
            id: 'user-1',
            login: 'alice',
            displayName: 'Alice'
          }),
          avatarUrl: ''
        })
      })
    );
    mocks.updateProfile.mockReset();
    mocks.updateProfile.mockImplementation((request: UpdateProfileRequest) =>
      Promise.resolve(
        new UpdateProfileResponse({
          profile: new ProfileSettingsView({
            user: new User({
              id: 'user-1',
              displayName: request.displayName ?? mocks.currentUser.user!.displayName,
              login: request.login ?? mocks.currentUser.user!.login
            }),
            avatarUrl: mocks.currentUser.user!.avatarUrl ?? ''
          })
        })
      )
    );
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

  it('submits a valid display name through the profile wire method', async () => {
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
      expect(mocks.updateProfile).toHaveBeenCalledOnce();
    });
    const request = mocks.updateProfile.mock.calls[0][0] as UpdateProfileRequest;
    expect(request.displayName).toBe('Ada Lovelace');
    expect(request.login).toBeUndefined();
    await expect.element(q(container, 'form')).toHaveTextContent('Profile updated successfully');
    await expect.element(displayNameInput).toHaveValue('Ada Lovelace');
  });

  it('shows client validation errors without calling the profile wire method', async () => {
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
});
