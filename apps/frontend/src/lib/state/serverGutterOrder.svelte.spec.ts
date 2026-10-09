import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ServerGutterOrder } from './serverGutterOrder.svelte';

const key = 'chatto:serverGutterOrder';
let stop: (() => void) | undefined;
beforeEach(() => localStorage.removeItem(key));
afterEach(() => {
  stop?.();
  stop = undefined;
  vi.restoreAllMocks();
});

describe('ServerGutterOrder', () => {
  it('loads saved order, drops unknown and duplicate IDs, and appends new servers', () => {
    localStorage.setItem(key, JSON.stringify(['b', 'gone', 'b', 'a']));
    expect(new ServerGutterOrder().ordered(['a', 'b', 'c'])).toEqual(['b', 'a', 'c']);
  });

  it.each(['invalid JSON', '{"a":1}', '["a",null]'])('rejects corrupt storage: %s', (raw) => {
    localStorage.setItem(key, raw);
    expect(new ServerGutterOrder().ordered(['a', 'b'])).toEqual(['a', 'b']);
  });

  it('keeps the formerly pinned origin first until a complete order is saved', () => {
    localStorage.setItem(key, JSON.stringify(['b', 'a']));
    const order = new ServerGutterOrder();
    expect(order.ordered(['a', 'origin', 'b'], 'origin')).toEqual(['origin', 'b', 'a']);
    order.move('origin', 1, ['a', 'origin', 'b'], 'origin');
    expect(JSON.parse(localStorage.getItem(key)!)).toEqual(['b', 'origin', 'a']);
    expect(new ServerGutterOrder().ordered(['a', 'origin', 'b'], 'origin')).toEqual([
      'b',
      'origin',
      'a'
    ]);
  });

  it('does not insert an unknown origin into a standalone frontend order', () => {
    localStorage.setItem(key, JSON.stringify(['b', 'a']));
    expect(new ServerGutterOrder().ordered(['a', 'b'], 'origin')).toEqual(['b', 'a']);
  });

  it('persists menu moves and keeps boundary moves unchanged', () => {
    const order = new ServerGutterOrder();
    order.move('b', -1, ['a', 'b', 'c']);
    expect(new ServerGutterOrder().ordered(['a', 'b', 'c'])).toEqual(['b', 'a', 'c']);
    order.move('b', -1, ['a', 'b', 'c']);
    order.move('missing', 1, ['a', 'b', 'c']);
    expect(order.ordered(['a', 'b', 'c'])).toEqual(['b', 'a', 'c']);
  });

  it('appends a removed server when it is added again, including after a reload', () => {
    const order = new ServerGutterOrder();
    order.save(['c', 'b', 'a']);
    order.forget('b');
    expect(order.ordered(['a', 'c'])).toEqual(['c', 'a']);
    expect(new ServerGutterOrder().ordered(['a', 'b', 'c'])).toEqual(['c', 'a', 'b']);
  });

  it('keeps local moves usable when storage writes fail', () => {
    const order = new ServerGutterOrder();
    stop = order.listen();
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('Full', 'QuotaExceededError');
    });
    order.move('c', -1, ['a', 'b', 'c']);
    order.move('c', -1, ['a', 'b', 'c']);
    window.dispatchEvent(new Event('focus'));
    expect(order.ordered(['a', 'b', 'c'])).toEqual(['c', 'a', 'b']);
  });

  it('ignores unrelated keys and session storage, catches resumed writes, and cleans up', () => {
    const order = new ServerGutterOrder();
    stop = order.listen();
    localStorage.setItem(key, JSON.stringify(['b', 'a']));
    window.dispatchEvent(new StorageEvent('storage', { key: 'other', storageArea: localStorage }));
    window.dispatchEvent(new StorageEvent('storage', { key, storageArea: sessionStorage }));
    expect(order.ordered(['a', 'b'])).toEqual(['a', 'b']);
    window.dispatchEvent(new Event('focus'));
    expect(order.ordered(['a', 'b'])).toEqual(['b', 'a']);
    stop();
    localStorage.setItem(key, JSON.stringify(['a', 'b']));
    window.dispatchEvent(new StorageEvent('storage', { key, storageArea: localStorage }));
    expect(order.ordered(['a', 'b'])).toEqual(['b', 'a']);
  });

  it('clears the server order when another tab clears storage', () => {
    const order = new ServerGutterOrder();
    order.save(['b', 'a']);
    stop = order.listen();
    localStorage.removeItem(key);
    window.dispatchEvent(new StorageEvent('storage', { key: null, storageArea: localStorage }));
    expect(order.ordered(['a', 'b'])).toEqual(['a', 'b']);
  });
});
