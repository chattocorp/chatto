import { beforeEach, describe, expect, it } from 'vitest';
import { availableCallDevice, CallPreferencesState } from './callPreferences.svelte';

describe('CallPreferencesState', () => {
  beforeEach(() => localStorage.clear());

  it('persists sensitivity per server and defaults older preferences to off', () => {
    const state = new CallPreferencesState('sensitivity');
    expect(state.microphoneThreshold).toBe(-60);
    state.setMicrophoneThreshold(-32);
    expect(new CallPreferencesState('sensitivity').microphoneThreshold).toBe(-32);
    expect(new CallPreferencesState('other').microphoneThreshold).toBe(-60);
    state.setMicrophoneThreshold(NaN);
    expect(new CallPreferencesState('sensitivity').microphoneThreshold).toBe(-60);
  });

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

it('persists fractional voice strength independently of gate, devices, join and server', () => {
  const state = new CallPreferencesState('voice-strength');
  state.setDevice('audioinput', 'chosen');
  state.setJoinMuted(true);
  state.setMicrophoneThreshold(-25);
  for (const amount of [0, 12.5, 50, 87.3, 100]) {
    state.setVoiceAmount(amount);
    const restored = new CallPreferencesState('voice-strength');
    expect(restored.voiceAmount).toBe(amount);
    expect(restored.effects.compressor).toBe(amount > 0);
    expect(restored.microphoneThreshold).toBe(-25);
    expect(restored.microphone).toBe('chosen');
    expect(restored.joinMuted).toBe(true);
    expect(new CallPreferencesState('separate-strength').voiceAmount).toBe(0);
  }
});

it.each([
  [{}, 0],
  [{ processingPreset: 'none' }, 0],
  [{ processingPreset: 'subtle' }, 50],
  [{ processingPreset: 'strong' }, 100],
  [{ processingPreset: 'invalid' }, 0],
  [{ effects: { compressor: true } }, 50],
  [{ effects: { compressor: 'true' } }, 0],
  [{ voiceAmount: 32.5, processingPreset: 'strong' }, 32.5],
  [{ voiceAmount: 'bad', processingPreset: 'strong' }, 0],
  [{ voiceAmount: null, processingPreset: 'strong' }, 0],
  [{ voiceAmount: 200 }, 100],
  [{ voiceAmount: -10 }, 0]
])('restores safe voice strength from %j', (saved, expected) => {
  localStorage.setItem(
    'chatto:i:migration:callPreferences',
    JSON.stringify({
      ...saved,
      microphone: 'mic',
      microphoneThreshold: -30
    })
  );
  const state = new CallPreferencesState('migration');
  expect(state.voiceAmount).toBe(expected);
  expect(state.microphone).toBe('mic');
  expect(state.microphoneThreshold).toBe(-30);
});

it('rejects non-finite runtime voice strength', () => {
  const state = new CallPreferencesState('invalid-strength');
  for (const value of [NaN, Infinity, -Infinity]) {
    state.setVoiceAmount(value);
    expect(state.voiceAmount).toBe(0);
  }
});

it('persists noise suppression independently and leaves it off for older preferences', () => {
  localStorage.clear();
  const state = new CallPreferencesState('noise');
  expect(state.noiseSuppression).toBe(false);
  state.setNoiseSuppression(true);
  expect(new CallPreferencesState('noise').effects.noiseSuppression).toBe(true);
  expect(state.voiceAmount).toBe(0);
  expect(new CallPreferencesState('other').noiseSuppression).toBe(false);
  state.setVoiceAmount(100);
  state.setNoiseSuppression(false);
  expect(new CallPreferencesState('noise').noiseSuppression).toBe(false);
  expect(state.voiceAmount).toBe(100);
});
