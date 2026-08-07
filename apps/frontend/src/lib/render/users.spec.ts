import { describe, expect, it, vi } from 'vitest';
import { buildDirectMessagePresentation } from './users';

const participants = [
  { id: 'self', login: 'me', displayName: 'Me' },
  { id: 'friend', login: 'friend', displayName: 'Friend' },
  { id: 'colleague', login: 'colleague', displayName: '' }
];

describe('buildDirectMessagePresentation', () => {
  it('builds a label and visible participant list from the other users', () => {
    const getDisplayName = vi.fn((_userId: string, fallback: string) => `Live ${fallback}`);

    expect(buildDirectMessagePresentation(participants, 'self', 'You', getDisplayName)).toEqual({
      label: 'Live Friend, Live colleague',
      visibleParticipants: participants.slice(1)
    });
    expect(getDisplayName).toHaveBeenNthCalledWith(1, 'friend', 'Friend');
    expect(getDisplayName).toHaveBeenNthCalledWith(2, 'colleague', 'colleague');
  });

  it('uses the localized current-user label and avatar for a self-DM', () => {
    expect(buildDirectMessagePresentation(participants.slice(0, 1), 'self', 'You')).toEqual({
      label: 'You',
      visibleParticipants: participants.slice(0, 1)
    });
  });

  it('keeps an empty participant list empty', () => {
    expect(buildDirectMessagePresentation([], 'self', 'You')).toEqual({
      label: 'You',
      visibleParticipants: []
    });
  });

  it('shows all participants while the current user is unknown', () => {
    expect(buildDirectMessagePresentation(participants, undefined, 'You')).toEqual({
      label: 'Me, Friend, colleague',
      visibleParticipants: participants
    });
  });
});
