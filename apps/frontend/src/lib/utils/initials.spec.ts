import { describe, it, expect } from 'vitest';
import { getAvatarInitials } from './initials';

describe('getAvatarInitials', () => {
  describe('with display name', () => {
    it('returns two initials for two-word names', () => {
      expect(getAvatarInitials('John Doe', 'johnd')).toBe('JD');
    });

    it('returns two initials for names with more than two words', () => {
      expect(getAvatarInitials('John Robert Doe', 'johnd')).toBe('JR');
    });

    it('returns single initial for single-word names', () => {
      expect(getAvatarInitials('Alice', 'alice')).toBe('A');
    });

    it('handles lowercase names', () => {
      expect(getAvatarInitials('john doe', 'johnd')).toBe('JD');
    });

    it('handles mixed case names', () => {
      expect(getAvatarInitials('jOhN dOe', 'johnd')).toBe('JD');
    });

    it('handles extra whitespace between words', () => {
      expect(getAvatarInitials('John    Doe', 'johnd')).toBe('JD');
    });

    it('handles leading and trailing whitespace', () => {
      expect(getAvatarInitials('  John Doe  ', 'johnd')).toBe('JD');
    });
  });

  describe('without display name', () => {
    it('falls back to first character of login', () => {
      expect(getAvatarInitials(null, 'johndoe')).toBe('J');
    });

    it('falls back to login when display name is undefined', () => {
      expect(getAvatarInitials(undefined, 'alice')).toBe('A');
    });

    it('falls back to login when display name is empty string', () => {
      expect(getAvatarInitials('', 'bob')).toBe('B');
    });

    it('falls back to login when display name is whitespace only', () => {
      expect(getAvatarInitials('   ', 'charlie')).toBe('C');
    });

    it('handles lowercase login', () => {
      expect(getAvatarInitials(null, 'david')).toBe('D');
    });
  });

  describe('edge cases', () => {
    it('returns ? when both display name and login are null', () => {
      expect(getAvatarInitials(null, null)).toBe('?');
    });

    it('returns ? when both display name and login are undefined', () => {
      expect(getAvatarInitials(undefined, undefined)).toBe('?');
    });

    it('returns ? when both are empty strings', () => {
      expect(getAvatarInitials('', '')).toBe('?');
    });

    it('returns ? when login is whitespace only and no display name', () => {
      expect(getAvatarInitials(null, '   ')).toBe('?');
    });
  });
});
