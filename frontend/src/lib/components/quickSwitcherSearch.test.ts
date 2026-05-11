import { describe, it, expect } from 'vitest';
import { scoreItem, type Searchable } from './quickSwitcherSearch';

const generalEng: Searchable = {
  label: 'general',
  detail: 'Engineering',
  serverName: 'Work'
};

const generalChatto: Searchable = {
  label: 'general',
  detail: 'Chatto Community',
  serverName: 'Chatto Community'
};

const random: Searchable = {
  label: 'random',
  detail: 'Engineering',
  serverName: 'Work'
};

describe('scoreItem', () => {
  it('returns null for an empty query', () => {
    expect(scoreItem('', generalEng)).toBeNull();
    expect(scoreItem('   ', generalEng)).toBeNull();
  });

  it('matches a single token against the label', () => {
    expect(scoreItem('general', generalEng)).not.toBeNull();
    expect(scoreItem('general', random)).toBeNull();
  });

  it('matches a single token against the space name (detail)', () => {
    expect(scoreItem('engineering', generalEng)).not.toBeNull();
  });

  it('matches a single token against the instance name', () => {
    expect(scoreItem('chatto', generalChatto)).not.toBeNull();
  });

  it('requires every token to match somewhere', () => {
    expect(scoreItem('general xyz', generalChatto)).toBeNull();
  });

  it('matches multi-token query across label and detail/instance', () => {
    // The motivating case: "general chatto" should match the room in
    // Chatto Community but not the one in Engineering / Work.
    expect(scoreItem('general chatto', generalChatto)).not.toBeNull();
    expect(scoreItem('general chatto', generalEng)).toBeNull();
  });

  it('is order-independent across tokens', () => {
    const forward = scoreItem('general chatto', generalChatto);
    const reverse = scoreItem('chatto general', generalChatto);
    expect(forward).not.toBeNull();
    expect(reverse).not.toBeNull();
    // Same tokens, same per-token best — total should match.
    expect(forward).toBe(reverse);
  });

  it('ranks label hits above detail hits above instance hits', () => {
    const labelHit: Searchable = { label: 'foo', detail: 'bar', serverName: 'baz' };
    const detailHit: Searchable = { label: 'bar', detail: 'foo', serverName: 'baz' };
    const instanceHit: Searchable = { label: 'bar', detail: 'baz', serverName: 'foo' };

    const labelScore = scoreItem('foo', labelHit)!;
    const detailScore = scoreItem('foo', detailHit)!;
    const instanceScore = scoreItem('foo', instanceHit)!;

    expect(labelScore).toBeGreaterThan(detailScore);
    expect(detailScore).toBeGreaterThan(instanceScore);
  });

  it('a label match beats a detail-only match for tie-breaking similar items', () => {
    // generalEng (label "general") vs generalChatto (label "general"); query
    // "general" alone — both match in label, scores equal.
    expect(scoreItem('general', generalEng)).toBe(scoreItem('general', generalChatto));
  });
});
