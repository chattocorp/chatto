import '../../../app.css';
import { userEvent } from 'vitest/browser';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import type { OutputAudioContext } from '$lib/state/server/callDeviceTest.svelte';
import { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
import CallDeviceSettings from './CallDeviceSettings.svelte';

const visibleCamera = {
  kind: 'videoinput',
  deviceId: 'camera',
  label: 'Camera'
} as MediaDeviceInfo;

const visibleMicrophone = {
  kind: 'audioinput',
  deviceId: 'microphone',
  label: 'Microphone'
} as MediaDeviceInfo;

describe('Call device settings', () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.restoreAllMocks());

  it.each([
    ['Microphone', 'microphone', 'microphone'],
    ['Speaker', 'speaker', 'speaker'],
    ['Camera', 'camera', 'camera']
  ] as const)(
    'commits %s chosen by clicking the enhanced native picker',
    async (label, id, field) => {
      vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
        visibleCamera,
        visibleMicrophone,
        { kind: 'audiooutput', deviceId: 'speaker', label: 'Speaker' } as MediaDeviceInfo
      ]);
      const preferences = new CallPreferencesState('pointer-device-selection');
      const screen = render(CallDeviceSettings, { preferences });
      const control = screen.getByRole('combobox', { name: label, exact: true });
      await control.click();
      await control.getByRole('option', { name: label, exact: true }).click();
      await expect.element(control).toHaveValue(id);
      expect(control.element().matches(':open')).toBe(false);
      expect(preferences[field]).toBe(id);
      expect(new CallPreferencesState('pointer-device-selection')[field]).toBe(id);
      await control.click();
      await control.getByRole('option', { name: 'System default' }).click();
      await expect.element(control).toHaveValue('');
      expect(new CallPreferencesState('pointer-device-selection')[field]).toBe('');
    }
  );

  it('saves each device selection and retains a removed camera as unavailable', async () => {
    const speaker = {
      kind: 'audiooutput',
      deviceId: 'speaker',
      label: 'Speakers'
    } as MediaDeviceInfo;
    const enumerate = vi
      .spyOn(navigator.mediaDevices, 'enumerateDevices')
      .mockResolvedValue([visibleCamera, visibleMicrophone, speaker]);
    vi.spyOn(HTMLMediaElement.prototype, 'setSinkId').mockResolvedValue();
    const preferences = new CallPreferencesState('select-devices');
    const screen = render(CallDeviceSettings, { preferences });
    await screen
      .getByRole('combobox', { name: 'Microphone', exact: true })
      .selectOptions('microphone');
    await screen.getByRole('combobox', { name: 'Speaker', exact: true }).selectOptions('speaker');
    const camera = screen.getByRole('combobox', { name: 'Camera', exact: true });
    await camera.selectOptions('camera');
    const saved = new CallPreferencesState('select-devices');
    expect([saved.microphone, saved.speaker, saved.camera]).toEqual([
      'microphone',
      'speaker',
      'camera'
    ]);
    enumerate.mockResolvedValue([visibleMicrophone, speaker]);
    navigator.mediaDevices.dispatchEvent(new Event('devicechange'));
    await expect
      .element(camera.getByRole('option', { name: 'Saved device unavailable' }))
      .toBeInTheDocument();
    await expect.element(camera).toHaveValue('camera');
    await camera.selectOptions('');
    expect(preferences.camera).toBe('');
  });

  it('shows the applied in-call device and restores it after a failed switch', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleMicrophone]);
    const preferences = new CallPreferencesState('committed-device');
    const screen = render(CallDeviceSettings, {
      preferences,
      inCall: true,
      onDeviceChange: async (kind, value) => {
        if (!value) throw new Error('Switch failed');
        preferences.setDevice(kind, value);
      }
    });
    const microphone = screen.getByRole('combobox', { name: 'Microphone', exact: true });
    await microphone.selectOptions('microphone');
    await expect.element(microphone).toHaveValue('microphone');
    await microphone.selectOptions('');
    await expect.element(microphone).toBeEnabled();
    await expect.element(microphone).toHaveValue('microphone');
    expect(preferences.microphone).toBe('microphone');
  });

  it('disables speaker selection when the browser has no output routing', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
      visibleCamera,
      visibleMicrophone
    ]);
    const mediaSink = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, 'setSinkId');
    const audioSink = Object.getOwnPropertyDescriptor(AudioContext.prototype, 'setSinkId');
    Reflect.deleteProperty(HTMLMediaElement.prototype, 'setSinkId');
    Reflect.deleteProperty(AudioContext.prototype, 'setSinkId');
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('no-output-routing')
    });
    try {
      const speaker = screen.getByRole('combobox', { name: 'Speaker', exact: true });
      await expect.element(speaker).toBeDisabled();
      await expect.element(speaker).toHaveAttribute('aria-describedby', 'call-speaker-description');
    } finally {
      await screen.unmount();
      if (mediaSink) Object.defineProperty(HTMLMediaElement.prototype, 'setSinkId', mediaSink);
      if (audioSink) Object.defineProperty(AudioContext.prototype, 'setSinkId', audioSink);
    }
  });

  it('persists threshold changes made with the keyboard', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
      visibleCamera,
      visibleMicrophone
    ]);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('threshold-control')
    });
    const slider = screen.container.querySelector<HTMLInputElement>('input[type=range]')!;
    slider.focus();
    await userEvent.keyboard('{Home}{ArrowRight}');
    expect(new CallPreferencesState('threshold-control').microphoneThreshold).toBe(-59);
  });

  it('routes in-call microphone changes through the active call instead of only saving a preference', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleMicrophone]);
    const preferences = new CallPreferencesState('active-device-switch');
    const change = vi.fn(async () => {});
    const screen = render(CallDeviceSettings, {
      preferences,
      inCall: true,
      onDeviceChange: change
    });
    await screen
      .getByRole('combobox', { name: 'Microphone', exact: true })
      .selectOptions('microphone');
    expect(change).toHaveBeenCalledWith('audioinput', 'microphone');
    // The call persists a successful switch; a failed switch must not claim this device.
    expect(preferences.microphone).toBe('');
    await expect
      .element(screen.getByRole('combobox', { name: 'Microphone', exact: true }))
      .toHaveValue('');
  });

  it('shows remembered unavailable devices without requesting capture', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
      visibleCamera,
      visibleMicrophone
    ]);
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    const preferences = new CallPreferencesState('settings-test');
    preferences.setDevice('audioinput', 'missing');
    const screen = render(CallDeviceSettings, { preferences });
    await expect
      .element(screen.getByRole('option', { name: 'Saved device unavailable' }))
      .toBeInTheDocument();
    expect(capture).not.toHaveBeenCalled();
    await screen.getByText('Join calls with microphone muted', { exact: true }).click();
    expect(new CallPreferencesState('settings-test').joinMuted).toBe(true);
    await screen.getByRole('combobox', { name: 'Microphone', exact: true }).selectOptions('');
    expect(new CallPreferencesState('settings-test').microphone).toBe('');
  });

  it('keeps live monitoring active when the device list refreshes', async () => {
    const enumerate = vi
      .spyOn(navigator.mediaDevices, 'enumerateDevices')
      .mockImplementation(async () => [visibleCamera, visibleMicrophone]);
    const context = new AudioContext();
    const oscillator = context.createOscillator();
    const destination = context.createMediaStreamDestination();
    oscillator.connect(destination);
    oscillator.start();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(async () => {
      await context.resume();
      return destination.stream;
    });
    const monitor = vi.spyOn(MicrophoneProcessor.prototype, 'connectMonitor');
    const preferences = new CallPreferencesState('playback-refresh');
    preferences.setVoiceBoosting(true);
    const screen = render(CallDeviceSettings, { preferences });
    try {
      await screen.getByRole('button', { name: 'Start microphone test' }).click();
      await expect.element(screen.getByRole('button', { name: 'Stop test' })).toBeInTheDocument();
      await vi.waitFor(() => expect(monitor).toHaveBeenCalledOnce());
      const output = monitor.mock.calls[0][0];
      const analyser = output.context.createAnalyser();
      (monitor.mock.contexts[0] as MicrophoneProcessor).connectMonitor(analyser);
      const samples = new Float32Array(analyser.fftSize);
      const expectOutput = () => {
        expect(output.context.state).toBe('running');
        analyser.getFloatTimeDomainData(samples);
        expect(Math.max(...samples.map(Math.abs))).toBeGreaterThan(0.1);
      };
      await vi.waitFor(expectOutput);
      const callsBefore = enumerate.mock.calls.length;
      navigator.mediaDevices.dispatchEvent(new Event('devicechange'));
      await vi.waitFor(() => expect(enumerate.mock.calls.length).toBeGreaterThan(callsBefore));
      await tick();
      expect(destination.stream.getAudioTracks()[0].readyState).toBe('live');
      await vi.waitFor(expectOutput);
    } finally {
      await screen.unmount();
      oscillator.stop();
      await context.close();
    }
  });

  it('requests camera access directly when camera names are hidden and releases it immediately', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([visibleMicrophone]);
    const stop = vi.fn();
    const capture = vi
      .spyOn(navigator.mediaDevices, 'getUserMedia')
      .mockResolvedValue({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('discover')
    });
    await vi.waitFor(() => expect(capture).toHaveBeenCalledWith({ video: true, audio: false }));
    expect(stop).toHaveBeenCalledOnce();
    expect(screen.container.textContent).not.toContain('Show cameras');
  });

  it('requests microphone access when only camera names are visible and refreshes audio choices', async () => {
    const enumerate = vi
      .spyOn(navigator.mediaDevices, 'enumerateDevices')
      .mockResolvedValueOnce([visibleCamera])
      .mockResolvedValue([
        visibleCamera,
        visibleMicrophone,
        { kind: 'audiooutput', deviceId: 'speaker', label: 'Speakers' } as MediaDeviceInfo
      ]);
    const stop = vi.fn();
    const capture = vi
      .spyOn(navigator.mediaDevices, 'getUserMedia')
      .mockResolvedValue({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('audio-discovery')
    });
    await expect
      .element(screen.getByRole('option', { name: 'Speakers', exact: true }))
      .toBeInTheDocument();
    await expect
      .element(screen.getByRole('option', { name: 'Microphone', exact: true }))
      .toBeInTheDocument();
    expect(capture).toHaveBeenCalledExactlyOnceWith({ audio: true, video: false });
    expect(stop).toHaveBeenCalledOnce();
    expect(enumerate).toHaveBeenCalledTimes(2);
  });

  it.each(['NotAllowedError', 'NotFoundError'])(
    'still discovers audio when camera capture fails with %s',
    async (name) => {
      vi.spyOn(navigator.mediaDevices, 'enumerateDevices')
        .mockResolvedValueOnce([])
        .mockResolvedValue([visibleMicrophone]);
      const stop = vi.fn();
      const capture = vi
        .spyOn(navigator.mediaDevices, 'getUserMedia')
        .mockResolvedValueOnce({ getTracks: () => [{ stop }] } as unknown as MediaStream)
        .mockRejectedValueOnce(new DOMException('Camera unavailable', name));
      const screen = render(CallDeviceSettings, {
        preferences: new CallPreferencesState('no-camera')
      });
      await vi.waitFor(() => expect(capture).toHaveBeenCalledTimes(2));
      await expect
        .element(screen.getByRole('option', { name: 'Microphone', exact: true }))
        .toBeInTheDocument();
      expect(stop).toHaveBeenCalledOnce();
    }
  );

  it('continues camera discovery after microphone permission is denied', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices')
      .mockResolvedValueOnce([])
      .mockResolvedValue([visibleCamera]);
    const stop = vi.fn();
    const capture = vi
      .spyOn(navigator.mediaDevices, 'getUserMedia')
      .mockRejectedValueOnce(new DOMException('Permission denied', 'NotAllowedError'))
      .mockResolvedValueOnce({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('denied-mic')
    });
    await expect
      .element(screen.getByRole('option', { name: 'Camera', exact: true }))
      .toBeInTheDocument();
    expect(capture).toHaveBeenCalledTimes(2);
    expect(stop).toHaveBeenCalledOnce();
    await expect.element(screen.getByText('Could not access a media device.')).toBeInTheDocument();
  });

  it('releases late discovery capture without requesting camera access after navigation', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([]);
    let resolve!: (stream: MediaStream) => void;
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('late-discovery')
    });
    await vi.waitFor(() => expect(capture).toHaveBeenCalledOnce());
    await screen.unmount();
    const stop = vi.fn();
    resolve({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    await vi.waitFor(() => expect(stop).toHaveBeenCalledOnce());
    expect(capture).toHaveBeenCalledOnce();
  });

  it('does not request discovery access during a call', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([]);
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    render(CallDeviceSettings, {
      preferences: new CallPreferencesState('busy-discovery'),
      inCall: true
    });
    await tick();
    expect(capture).not.toHaveBeenCalled();
  });

  it('blocks microphone testing during a call', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
      visibleCamera,
      visibleMicrophone
    ]);
    const screen = render(CallDeviceSettings, {
      preferences: new CallPreferencesState('busy'),
      inCall: true
    });
    await expect
      .element(screen.getByRole('button', { name: 'Start microphone test' }))
      .toBeDisabled();
  });

  it('releases capture that resolves after the settings page closes', async () => {
    vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
      visibleCamera,
      visibleMicrophone
    ]);
    let resolve!: (stream: MediaStream) => void;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const stop = vi.fn();
    const screen = render(CallDeviceSettings, { preferences: new CallPreferencesState('close') });
    await screen.getByRole('button', { name: 'Start microphone test' }).click();
    await screen.unmount();
    resolve({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    await vi.waitFor(() => expect(stop).toHaveBeenCalledOnce());
  });
});

it('offers default-on voice boosting with a persistent keyboard opt-out and separate gate', async () => {
  vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
    visibleCamera,
    visibleMicrophone
  ]);
  const preferences = new CallPreferencesState('voice-ui');
  preferences.setMicrophoneThreshold(-30);
  const screen = render(CallDeviceSettings, { preferences });
  const checkbox = screen.getByRole('checkbox', { name: 'Voice Boosting' });
  await expect.element(checkbox).toBeChecked();
  await expect.element(checkbox).toHaveAccessibleDescription(
    'Enhance your microphone audio. Turn this off if it causes audio problems.'
  );
  checkbox.element().focus();
  await userEvent.keyboard(' ');
  await expect.element(checkbox).not.toBeChecked();
  expect(new CallPreferencesState('voice-ui').voiceBoosting).toBe(false);
  expect(preferences.effects.compressor).toBe(false);
  await userEvent.keyboard(' ');
  await expect.element(checkbox).toBeChecked();
  expect(new CallPreferencesState('voice-ui').voiceBoosting).toBe(true);
  expect(preferences.effects.treble).toBe(10);
  expect(preferences.microphoneThreshold).toBe(-30);
  expect(screen.container.querySelectorAll('input[type=range]')).toHaveLength(1);
});

it('disables voice boosting when processing is unavailable', async () => {
  vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
    visibleCamera,
    visibleMicrophone
  ]);
  const screen = render(CallDeviceSettings, {
    preferences: new CallPreferencesState('unavailable-voice'),
    inCall: true,
    gateUnavailable: true
  });
  await expect.element(screen.getByRole('checkbox', { name: 'Voice Boosting' })).toBeDisabled();
});

it('switches the selected test output without stopping capture or requiring another Start click', async () => {
  const context = new AudioContext();
  const stream = context.createMediaStreamDestination().stream;
  const preferences = new CallPreferencesState('live-output');
  preferences.setVoiceBoosting(true);
  const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
  const sink = vi
    .spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId')
    .mockResolvedValue(undefined);
  vi.spyOn(navigator.mediaDevices, 'enumerateDevices').mockResolvedValue([
    visibleCamera,
    visibleMicrophone,
    { kind: 'audiooutput', deviceId: 'headphones', label: 'Headphones' } as MediaDeviceInfo
  ]);
  const screen = render(CallDeviceSettings, { preferences });
  try {
    await screen.getByRole('button', { name: 'Start microphone test' }).click();
    await expect.element(screen.getByRole('button', { name: 'Stop test' })).toBeInTheDocument();
    // Stop is available during startup too. Switch output only once capture
    // and the audio graph are ready; a pending device change restarts the test.
    await expect.element(screen.getByText('Waiting for microphone access…')).not.toBeInTheDocument();
    await screen
      .getByRole('combobox', { name: 'Speaker', exact: true })
      .selectOptions('headphones');
    await vi.waitFor(() => expect(sink).toHaveBeenLastCalledWith('headphones'));
    await vi.waitFor(() => expect(preferences.speaker).toBe('headphones'));
    expect(capture).toHaveBeenCalledOnce();
    expect(stream.getAudioTracks()[0].readyState).toBe('live');
    await expect.element(screen.getByRole('button', { name: 'Stop test' })).toBeInTheDocument();
    await screen.getByRole('combobox', { name: 'Speaker', exact: true }).selectOptions('');
    await vi.waitFor(() => expect(sink).toHaveBeenLastCalledWith(''));
    expect(capture).toHaveBeenCalledOnce();
  } finally {
    await screen.unmount();
    await context.close();
    vi.restoreAllMocks();
  }
});
