import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@capacitor/core', () => ({ Capacitor: { getPlatform: () => 'ios' } }));

import { UserPreferencesState } from './userPreferences.svelte';

describe('native iOS surface depth', () => {
  beforeEach(() => localStorage.clear());

  it('defaults to Flat for a new installation', () => {
    expect(new UserPreferencesState().surfaceDepth).toBe(0);
  });

  it('defaults to Flat when existing preferences have no depth setting', () => {
    localStorage.setItem('chatto:preferences', JSON.stringify({ accentColor: 'blue' }));
    expect(new UserPreferencesState().surfaceDepth).toBe(0);
  });

  it.each([
    ['flat', 0],
    ['3d', 50],
    ['very-3d', 100],
    [70, 70]
  ])('preserves the saved %s setting', (surfaceDepth, level) => {
    localStorage.setItem('chatto:preferences', JSON.stringify({ surfaceDepth }));
    expect(new UserPreferencesState().surfaceDepth).toBe(level);
  });
});
