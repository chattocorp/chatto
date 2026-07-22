import { describe, expect, it } from 'vitest';
import { usesAppleShortcutModifier } from './platformShortcuts';

describe('usesAppleShortcutModifier', () => {
  it.each(['MacIntel', 'iPhone', 'iPad', 'iPod'])('recognises %s as an Apple platform', (platform) => {
    expect(usesAppleShortcutModifier({ platform })).toBe(true);
  });

  it.each(['Win32', 'Linux x86_64', ''])('does not use Command shortcuts on %s', (platform) => {
    expect(usesAppleShortcutModifier({ platform })).toBe(false);
  });

  it('prefers User-Agent Client Hints when available', () => {
    expect(
      usesAppleShortcutModifier({
        platform: 'MacIntel',
        userAgentData: { platform: 'Windows' }
      })
    ).toBe(false);
  });
});
