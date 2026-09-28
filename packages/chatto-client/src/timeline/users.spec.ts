import { describe, expect, it, vi } from 'vitest';
import { buildDirectMessagePresentation, deletedDirectMessageParticipant } from './users.js';

const labels = { currentUser: 'You', deletedUser: '[deleted user]' };

const participants = [
  { id: 'self', login: 'me', displayName: 'Me' },
  { id: 'friend', login: 'friend', displayName: 'Friend' },
  { id: 'colleague', login: 'colleague', displayName: '' }
];

describe('buildDirectMessagePresentation', () => {
  it('builds a label and visible participant list from the other users', () => {
    const getDisplayName = vi.fn((_userId: string, fallback: string) => `Live ${fallback}`);

    expect(buildDirectMessagePresentation(participants, 'self', labels, getDisplayName)).toEqual({
      label: 'Live Friend, Live colleague',
      visibleParticipants: participants.slice(1)
    });
    expect(getDisplayName).toHaveBeenNthCalledWith(1, 'friend', 'Friend');
    expect(getDisplayName).toHaveBeenNthCalledWith(2, 'colleague', 'colleague');
  });

  it('uses the live display name and localized current-user suffix for a self-DM', () => {
    const getDisplayName = vi.fn(() => 'Updated Me');
    expect(
      buildDirectMessagePresentation(participants.slice(0, 1), 'self', labels, getDisplayName)
    ).toEqual({
      label: 'Updated Me (You)',
      visibleParticipants: participants.slice(0, 1)
    });
    expect(getDisplayName).toHaveBeenCalledWith('self', 'Me');
  });

  it('falls back to the login when the self participant has no display name', () => {
    expect(
      buildDirectMessagePresentation([{ ...participants[0], displayName: '' }], 'self', labels)
        .label
    ).toBe('me (You)');
    expect(
      buildDirectMessagePresentation(participants.slice(0, 1), 'self', labels, () => '').label
    ).toBe('me (You)');
  });

  it('keeps an empty participant list empty', () => {
    expect(buildDirectMessagePresentation([], 'self', labels)).toEqual({
      label: 'You',
      visibleParticipants: []
    });
  });

  it('shows all participants while the current user is unknown', () => {
    expect(buildDirectMessagePresentation(participants, undefined, labels)).toEqual({
      label: 'Me, Friend, colleague',
      visibleParticipants: participants
    });
  });

  it('shows a deleted partner instead of turning the DM into a self-DM', () => {
    const deleted = deletedDirectMessageParticipant('gone');
    const getDisplayName = vi.fn((_userId: string, fallback: string) => fallback);
    expect(
      buildDirectMessagePresentation([participants[0], deleted], 'self', labels, getDisplayName)
    ).toEqual({
      label: '[deleted user]',
      visibleParticipants: [deleted]
    });
    expect(getDisplayName).not.toHaveBeenCalled();
  });
});
