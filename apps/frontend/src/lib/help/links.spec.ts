import { describe, expect, it } from 'vitest';
import { withHelpServer } from './links';
import { helpGroups } from './navigation';

describe('Help account selection in links', () => {
  it('keeps public links unchanged without an explicit server', () => {
    expect(withHelpServer('/help/conversations#section-threads')).toBe(
      '/help/conversations#section-threads'
    );
  });
  it('preserves section anchors and other query values while replacing server selection', () => {
    expect(
      withHelpServer('/help/conversations?server=old&view=full#section-threads', 'remote.example')
    ).toBe('/help/conversations?server=remote.example&view=full#section-threads');
  });
  it('encodes a server as one query value', () => {
    expect(withHelpServer('/help', 'remote&view=wrong#section')).toBe(
      '/help?server=remote%26view%3Dwrong%23section'
    );
  });
  it('keeps the selection on every overview and related-guide destination', () => {
    const items = helpGroups('remote.example').flatMap((group) => group.items);
    expect(items).toHaveLength(4);
    for (const item of items) expect(item.href).toBe(`/help/${item.topic}?server=remote.example`);
  });
});
