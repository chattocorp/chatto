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

it('persists each preset without changing the gate, devices or join choice', () => {
  const state = new CallPreferencesState('presets');
  state.setDevice('audioinput', 'chosen');
  state.setJoinMuted(true);
  state.setMicrophoneThreshold(-25);
  for (const preset of ['subtle', 'strong', 'none'] as const) {
    state.setProcessingPreset(preset);
    const restored = new CallPreferencesState('presets');
    expect(restored.processingPreset).toBe(preset);
    expect(new CallPreferencesState('separate-presets').processingPreset).toBe('none');
    expect(restored.effects.compressor).toBe(preset !== 'none');
    expect(restored.effects.equalizer).toBe(preset !== 'none');
    expect(restored.effects.lowCut).toBe(preset !== 'none');
    expect(restored.microphoneThreshold).toBe(-25);
    expect(restored.microphone).toBe('chosen');
    expect(restored.joinMuted).toBe(true);
  }
});

it.each([
  [{}, 'none'],
  [{ processingPreset: 'invalid', effects: { compressor: true } }, 'none'],
  [{ processingPreset: 'strong' }, 'strong'],
  [{ effects: { lowCut: false, equalizer: false, compressor: false } }, 'none'],
  [{ effects: { lowCut: true } }, 'subtle'],
  [{ effects: { equalizer: true } }, 'subtle'],
  [{ effects: { compressor: true } }, 'subtle'],
  [{ effects: { compressor: 'true' } }, 'none']
])('restores safe presets from saved preferences %j', (saved, expected) => {
  localStorage.setItem(
    'chatto:i:migration:callPreferences',
    JSON.stringify({
      ...saved,
      microphone: 'mic',
      microphoneThreshold: -30
    })
  );
  const state = new CallPreferencesState('migration');
  expect(state.processingPreset).toBe(expected);
  expect(state.microphone).toBe('mic');
  expect(state.microphoneThreshold).toBe(-30);
});

it('uses stronger tone shaping and compression for Strong than Subtle', () => {
  const state = new CallPreferencesState('strength');
  state.setProcessingPreset('subtle');
  const subtle = state.effects;
  state.setProcessingPreset('strong');
  expect(state.effects.amount).toBeGreaterThan(subtle.amount);
  expect(state.effects.treble).toBeGreaterThan(subtle.treble);
  state.effects.bass = 6;
  expect(state.effects.bass).toBe(-3);
});
