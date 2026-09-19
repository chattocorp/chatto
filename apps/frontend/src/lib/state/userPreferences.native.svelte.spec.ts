import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@capacitor/core', () => ({ Capacitor: { getPlatform: () => 'ios' } }));

import { UserPreferencesState } from './userPreferences.svelte';

describe('native iOS surface depth', () => {
  beforeEach(() => localStorage.clear());

  it('defaults to Flat for a new installation', () => {
    expect(new UserPreferencesState().surfaceDepth).toBe('flat');
  });

  it('defaults to Flat when existing preferences have no depth setting', () => {
    localStorage.setItem('chatto:preferences', JSON.stringify({ accentColor: 'blue' }));
    expect(new UserPreferencesState().surfaceDepth).toBe('flat');
  });

  it.each(['flat', '3d', 'very-3d'])('preserves the saved %s setting', (surfaceDepth) => {
    localStorage.setItem('chatto:preferences', JSON.stringify({ surfaceDepth }));
    expect(new UserPreferencesState().surfaceDepth).toBe(surfaceDepth);
  });
});
