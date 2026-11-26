import { afterEach, expect, it, vi } from 'vitest';
import { bindDesktopMuteShortcut } from './muteShortcut';

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
  const dispose = bindDesktopMuteShortcut('Alt+M', toggle)!;
  const [[chord, token]] = set.mock.calls;
  expect(chord).toBe('Alt+M');
  press('previous');
  press(token);
  dispose();
  press(token);
  expect(toggle).toHaveBeenCalledTimes(1);
  expect(release).toHaveBeenCalledOnce();
  expect(set).toHaveBeenLastCalledWith('', token);
});
