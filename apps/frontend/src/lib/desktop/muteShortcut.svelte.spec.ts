import { afterEach, expect, it, vi } from 'vitest';
import { bindDesktopMuteShortcut, bindDesktopPushToTalk } from './muteShortcut';

afterEach(() => {
  delete window.chattoDesktop;
});

it('delivers only this call’s presses and releases the binding on exit', () => {
  let press: (token: string) => void = () => {};
  const release = vi.fn();
  const set = vi.fn(async (_chord: string, _token: string) => true);
  window.chattoDesktop = {
    muteShortcut: {
      available: vi.fn(),
      set,
      onPress: (callback) => {
        press = callback;
        return release;
      }
    }
  };
  const toggle = vi.fn();
  expect(bindDesktopMuteShortcut('', toggle)).toBeUndefined();
  const dispose = bindDesktopMuteShortcut('Super+M', toggle)!;
  const [[chord, token]] = set.mock.calls;
  expect(chord).toBe('Super+M');
  press('previous');
  press(token);
  dispose();
  press(token);
  expect(toggle).toHaveBeenCalledTimes(1);
  expect(release).toHaveBeenCalledOnce();
  expect(set).toHaveBeenLastCalledWith('', token);
});

it('ends a hold on disposal and ignores late edges from an old call', () => {
  let edge: (token: string, value: 'up' | 'down' | 'failed') => void = () => {};
  const unsubscribe = vi.fn();
  const set = vi.fn(async (_chord: string, _token: string) => true);
  window.chattoDesktop = {
    pushToTalk: {
      set,
      onEdge: (callback) => {
        edge = callback;
        return unsubscribe;
      }
    }
  };
  const setHeld = vi.fn();
  expect(bindDesktopPushToTalk('', setHeld)).toBeUndefined();
  expect(set).not.toHaveBeenCalled();
  const dispose = bindDesktopPushToTalk('Control+Super+M', setHeld)!;
  const [[, token]] = set.mock.calls;
  edge('previous call', 'down');
  edge(token, 'down');
  dispose();
  edge(token, 'down');
  expect(setHeld.mock.calls).toEqual([[true], [false]]);
  expect(unsubscribe).toHaveBeenCalledOnce();
  expect(set).toHaveBeenLastCalledWith('', token);
});
