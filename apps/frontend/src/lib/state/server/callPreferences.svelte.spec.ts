import { beforeEach, describe, expect, it } from 'vitest';
import { availableCallDevice, CallPreferencesState } from './callPreferences.svelte';

describe('CallPreferencesState', () => {
  beforeEach(() => localStorage.clear());

  it('persists independent participant levels by server and restores defaults on reset', () => {
    const state = new CallPreferencesState('participant-audio');
    state.setParticipantVolume('bob', 'voiceVolume', 175);
    state.setParticipantVolume('bob', 'streamVolume', 30);
    state.setParticipantVolume('alice', 'voiceVolume', 0);
    const restored = new CallPreferencesState('participant-audio');
    expect(restored.getParticipantAudio('bob')).toEqual({ voiceVolume: 175, streamVolume: 30 });
    expect(restored.getParticipantAudio('alice')).toEqual({ voiceVolume: 0, streamVolume: 100 });
    expect(new CallPreferencesState('other').getParticipantAudio('bob').voiceVolume).toBe(100);
    restored.resetParticipantAudio('bob');
    expect(new CallPreferencesState('participant-audio').getParticipantAudio('bob')).toEqual({
      voiceVolume: 100,
      streamVolume: 100
    });
    expect(restored.getParticipantAudio('alice').voiceVolume).toBe(0);
  });

  it('bounds participant volume and rejects non-finite levels', () => {
    const state = new CallPreferencesState('audio-bounds');
    state.setParticipantVolume('bob', 'voiceVolume', 900);
    state.setParticipantVolume('bob', 'streamVolume', -10);
    expect(state.getParticipantAudio('bob')).toEqual({ voiceVolume: 200, streamVolume: 0 });
    state.setParticipantVolume('bob', 'voiceVolume', NaN);
    state.setParticipantVolume('bob', 'streamVolume', Infinity);
    expect(state.getParticipantAudio('bob')).toEqual({ voiceVolume: 100, streamVolume: 100 });
  });

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

it('defaults voice boosting to on and persists opt-out independently of other preferences', () => {
  const state = new CallPreferencesState('voice-boosting');
  expect(state.voiceBoosting).toBe(true);
  expect(state.effects.polish).toBe(1);
  state.setDevice('audioinput', 'chosen');
  state.setDevice('audiooutput', 'speaker');
  state.setDevice('videoinput', 'camera');
  state.setJoinMuted(true);
  state.setMicrophoneThreshold(-25);
  state.setParticipantVolume('bob', 'voiceVolume', 125);
  for (const enabled of [false, true]) {
    state.setVoiceBoosting(enabled);
    const restored = new CallPreferencesState('voice-boosting');
    expect(restored.voiceBoosting).toBe(enabled);
    expect(restored.effects.compressor).toBe(enabled);
    expect(restored.effects.polish).toBe(enabled ? 1 : 0);
    expect(restored.microphoneThreshold).toBe(-25);
    expect([restored.microphone, restored.speaker, restored.camera]).toEqual([
      'chosen', 'speaker', 'camera'
    ]);
    expect(restored.joinMuted).toBe(true);
    expect(restored.getParticipantAudio('bob').voiceVolume).toBe(125);
    expect(new CallPreferencesState('separate-boosting').voiceBoosting).toBe(true);
  }
});

it.each([
  {},
  ...['none', 'subtle', 'strong', 'invalid'].map((processingPreset) => ({ processingPreset })),
  ...[true, false, 'true'].map((compressor) => ({ effects: { compressor } })),
  { effects: { lowCut: true } },
  { effects: { equalizer: true } },
  ...[0, 32.5, 50, 100, 200, -10, 'bad', null].map((voiceAmount) => ({ voiceAmount })),
  ...[null, 0, 1, 'false', {}, []].map((voiceBoosting) => ({ voiceBoosting }))
])('enables boosting for legacy or invalid preferences %j', (saved) => {
  localStorage.setItem(
    'chatto:i:migration:callPreferences',
    JSON.stringify({ ...saved, microphone: 'mic', microphoneThreshold: -30 })
  );
  const state = new CallPreferencesState('migration');
  expect(state.voiceBoosting).toBe(true);
  expect(state.effects.polish).toBe(1);
  expect(state.microphone).toBe('mic');
  expect(state.microphoneThreshold).toBe(-30);
});

it('keeps an explicit opt-out even when old processing fields remain', () => {
  localStorage.setItem(
    'chatto:i:opt-out:callPreferences',
    JSON.stringify({
      voiceBoosting: false,
      voiceAmount: 100,
      processingPreset: 'strong',
      effects: { compressor: true }
    })
  );
  const state = new CallPreferencesState('opt-out');
  expect(state.voiceBoosting).toBe(false);
  expect(state.effects.polish).toBe(0);
  state.setJoinMuted(true);
  const saved = JSON.parse(localStorage.getItem('chatto:i:opt-out:callPreferences')!);
  expect(saved.voiceBoosting).toBe(false);
  expect(saved).not.toHaveProperty('voiceAmount');
  expect(saved).not.toHaveProperty('processingPreset');
  expect(saved).not.toHaveProperty('effects');
});
