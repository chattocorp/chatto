import { beforeEach, describe, expect, it } from 'vitest';
import { availableCallDevice, CallPreferencesState } from './callPreferences.svelte';

describe('CallPreferencesState', () => {
  beforeEach(() => localStorage.clear());

  it('restores devices and join-muted independently for each server', () => {
    const first = new CallPreferencesState('first');
    first.setDevice('audioinput', 'mic');
    first.setDevice('audiooutput', 'speaker');
    first.setDevice('videoinput', 'camera');
    first.setJoinMuted(true);
    const restored = new CallPreferencesState('first');
    expect([restored.microphone, restored.speaker, restored.camera, restored.joinMuted]).toEqual([
      'mic',
      'speaker',
      'camera',
      true
    ]);
    const other = new CallPreferencesState('other');
    expect([other.microphone, other.speaker, other.camera, other.joinMuted]).toEqual([
      '',
      '',
      '',
      false
    ]);
  });

  it('retains unavailable devices for their return and supports resetting to default', () => {
    const state = new CallPreferencesState('first');
    state.setDevice('audioinput', 'mic');
    expect(availableCallDevice(state.microphone, [])).toBe('');
    expect(state.microphone).toBe('mic');
    expect(availableCallDevice(state.microphone, [{ deviceId: 'mic' } as MediaDeviceInfo])).toBe(
      'mic'
    );
    state.setDevice('audioinput', '');
    expect(new CallPreferencesState('first').microphone).toBe('');
  });

  it('normalizes corrupt and partially invalid storage without enabling capture', () => {
    localStorage.setItem(
      'chatto:i:first:callPreferences',
      '{"microphone":5,"camera":"cam","joinMuted":"true"}'
    );
    const state = new CallPreferencesState('first');
    expect(state.microphone).toBe('');
    expect(state.camera).toBe('cam');
    expect(state.joinMuted).toBe(false);
    localStorage.setItem('chatto:i:first:callPreferences', 'invalid');
    expect(new CallPreferencesState('first').camera).toBe('');
  });
});
