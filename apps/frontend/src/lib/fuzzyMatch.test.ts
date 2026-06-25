/**
 * Unit tests for fuzzy matching utility.
 */
import { describe, it, expect } from 'vitest';
import { fuzzyMatch } from './fuzzyMatch';

describe('fuzzyMatch', () => {
  describe('exact matches', () => {
    it('returns highest score for exact match', () => {
      expect(fuzzyMatch('foobar', 'foobar')).toBe(1000);
    });

    it('is case-insensitive for exact match', () => {
      expect(fuzzyMatch('FooBar', 'foobar')).toBe(1000);
      expect(fuzzyMatch('foobar', 'FOOBAR')).toBe(1000);
    });
  });

  describe('prefix matches', () => {
    it('returns high score for prefix match', () => {
      const score = fuzzyMatch('foo', 'foobar');
      expect(score).not.toBeNull();
      expect(score!).toBeGreaterThan(500);
      expect(score!).toBeLessThan(1000);
    });

    it('is case-insensitive for prefix match', () => {
      const score = fuzzyMatch('FOO', 'foobar');
      expect(score).not.toBeNull();
      expect(score!).toBeGreaterThan(500);
    });

    it('gives higher score for longer prefix coverage', () => {
      const shortPrefix = fuzzyMatch('f', 'foobar')!;
      const longPrefix = fuzzyMatch('fooba', 'foobar')!;
      expect(longPrefix).toBeGreaterThan(shortPrefix);
    });
  });

  describe('contains matches', () => {
    it('returns positive score for contains match', () => {
      const score = fuzzyMatch('bar', 'foobar');
      expect(score).not.toBeNull();
      expect(score!).toBeGreaterThan(0);
    });

    it('contains match scores lower than prefix match', () => {
      const prefixScore = fuzzyMatch('foo', 'foobar')!;
      const containsScore = fuzzyMatch('bar', 'foobar')!;
      expect(prefixScore).toBeGreaterThan(containsScore);
    });

    it('is case-insensitive for contains match', () => {
      const score = fuzzyMatch('BAR', 'foobar');
      expect(score).not.toBeNull();
      expect(score!).toBeGreaterThan(0);
    });

    it('gives higher score for earlier position', () => {
      const earlyMatch = fuzzyMatch('oo', 'foobar')!;
      const lateMatch = fuzzyMatch('ar', 'foobar')!;
      expect(earlyMatch).toBeGreaterThan(lateMatch);
    });
  });

  describe('fuzzy subsequence matches', () => {
    it('matches characters in order', () => {
      const score = fuzzyMatch('fb', 'foobar');
      expect(score).not.toBeNull();
      expect(score!).toBeGreaterThan(0);
    });

    it('returns null for characters out of order', () => {
      expect(fuzzyMatch('bf', 'foobar')).toBeNull();
    });

    it('matches across word boundaries', () => {
      const score = fuzzyMatch('jd', 'john.doe');
      expect(score).not.toBeNull();
      expect(score!).toBeGreaterThan(0);
    });

    it('gives bonus for word boundary matches', () => {
      // 'jd' in 'john.doe': j at start (boundary), d after '.' (boundary)
      // 'je' in 'john.doe': j at start (boundary), e mid-word (no boundary)
      // Both are fuzzy matches (not substrings), so we can compare fuzzy scores
      const boundaryMatch = fuzzyMatch('jd', 'john.doe')!;
      const partialBoundaryMatch = fuzzyMatch('je', 'john.doe')!;
      expect(boundaryMatch).toBeGreaterThan(partialBoundaryMatch);
    });

    it('prefix matches score higher than fuzzy matches with gaps', () => {
      // 'fo' is a prefix match (scores 500+), 'fb' is a fuzzy match with a gap
      const prefix = fuzzyMatch('fo', 'foobar')!;
      const fuzzyWithGap = fuzzyMatch('fb', 'foobar')!;
      expect(prefix).toBeGreaterThan(fuzzyWithGap);
    });

    it('fuzzy score is lower than contains score', () => {
      const containsScore = fuzzyMatch('oob', 'foobar')!;
      const fuzzyScore = fuzzyMatch('fbr', 'foobar')!;
      expect(containsScore).toBeGreaterThan(fuzzyScore);
    });

    it('is case-insensitive for fuzzy match', () => {
      const score = fuzzyMatch('FB', 'foobar');
      expect(score).not.toBeNull();
      expect(score!).toBeGreaterThan(0);
    });
  });

  describe('non-matches', () => {
    it('returns null when character is missing', () => {
      expect(fuzzyMatch('xyz', 'foobar')).toBeNull();
    });

    it('returns null for partial character match', () => {
      expect(fuzzyMatch('fooz', 'foobar')).toBeNull();
    });

    it('returns null for empty query', () => {
      expect(fuzzyMatch('', 'foobar')).toBeNull();
    });

    it('returns null for empty target', () => {
      expect(fuzzyMatch('foo', '')).toBeNull();
    });

    it('returns null for query longer than target', () => {
      expect(fuzzyMatch('foobarx', 'foobar')).toBeNull();
    });
  });

  describe('score ordering', () => {
    it('orders: exact > prefix > contains > fuzzy', () => {
      const exact = fuzzyMatch('foobar', 'foobar')!;
      const prefix = fuzzyMatch('foo', 'foobar')!;
      const contains = fuzzyMatch('oob', 'foobar')!;
      const fuzzy = fuzzyMatch('fbr', 'foobar')!;

      expect(exact).toBeGreaterThan(prefix);
      expect(prefix).toBeGreaterThan(contains);
      expect(contains).toBeGreaterThan(fuzzy);
    });
  });

  describe('real-world usernames', () => {
    it('matches typical usernames', () => {
      expect(fuzzyMatch('alice', 'alice123')).not.toBeNull();
      expect(fuzzyMatch('al', 'alice123')).not.toBeNull();
      expect(fuzzyMatch('a1', 'alice123')).not.toBeNull();
    });

    it('matches usernames with dots', () => {
      expect(fuzzyMatch('hm', 'hendrik.mans')).not.toBeNull();
      expect(fuzzyMatch('mans', 'hendrik.mans')).not.toBeNull();
    });

    it('matches initials-style queries', () => {
      expect(fuzzyMatch('tu', 'testuser123')).not.toBeNull();
    });
  });
});
