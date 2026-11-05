import { afterEach, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { CallAudioLevels } from './callAudioLevels';

const cleanups: Array<() => void | Promise<void>> = [];
afterEach(async () => {
  try {
    for (const cleanup of cleanups.splice(0)) await cleanup();
  } finally {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  }
});

/** Produce silent, browser-local media without requesting microphone access or playing sound. */
async function signal() {
  const context = new AudioContext();
  const oscillator = context.createOscillator();
  const gain = context.createGain();
  gain.gain.value = 0;
  const destination = context.createMediaStreamDestination();
  oscillator.connect(gain).connect(destination);
  oscillator.start();
  const button = document.createElement('button');
  document.body.append(button);
  button.onclick = () => {
    void context.resume().catch(() => {});
  };
  await userEvent.click(button);
  await context.resume();
  button.remove();
  cleanups.push(async () => {
    oscillator.stop();
    destination.stream.getTracks().forEach((track) => track.stop());
    await context.close();
  });
  return {
    context,
    gain,
    stream: destination.stream,
    track: destination.stream.getAudioTracks()[0]
  };
}

function levels() {
  const levels = new CallAudioLevels();
  cleanups.unshift(() => levels.clear());
  return levels;
}

it('measures raw capture before processing, shares one context, and releases only analysis on replacement', async () => {
  const first = await signal();
  const second = await signal();
  const create = vi.spyOn(AudioContext.prototype, 'createMediaStreamSource');
  const meters = levels();
  const capture = { mediaStream: first.stream, mediaStreamTrack: second.track };
  const remote = { mediaStreamTrack: second.track };
  const update = () =>
    meters.update([
      ['local', capture],
      ['remote', remote]
    ]);
  first.gain.gain.value = 0.2;
  await expect
    .poll(() => {
      update();
      return meters.get('local') ?? 0;
    })
    .toBeGreaterThan(0.1);
  expect(meters.get('remote')).toBe(0);
  expect(create).toHaveBeenCalledTimes(2);
  expect(create.mock.contexts[0]).toBe(create.mock.contexts[1]);
  capture.mediaStream = second.stream;
  await expect
    .poll(() => {
      update();
      return meters.get('local');
    })
    .toBe(0);
  first.gain.gain.value = 0;
  second.gain.gain.value = 0.1;
  await expect
    .poll(() => {
      update();
      return meters.get('remote') ?? 0;
    })
    .toBeGreaterThan(0.05);
  const close = vi.spyOn(AudioContext.prototype, 'close');
  meters.update([]);
  meters.clear();
  expect(close).toHaveBeenCalledOnce();
  expect(first.track.readyState).toBe('live');
  expect(second.track.readyState).toBe('live');
  expect(meters.get('local')).toBeUndefined();
});

it('silences disabled tracks and releases meters in a hidden document', async () => {
  const { track, gain } = await signal();
  const meters = levels();
  const update = () => meters.update([['local', { mediaStreamTrack: track }]]);
  gain.gain.value = 0.2;
  await expect
    .poll(() => {
      update();
      return meters.get('local') ?? 0;
    })
    .toBeGreaterThan(0.1);
  track.enabled = false;
  update();
  expect(meters.get('local')).toBe(0);
  track.enabled = true;
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
  update();
  expect(meters.get('local')).toBeUndefined();
  expect(track.readyState).toBe('live');
});

it('keeps source tracks usable when optional analysis fails', async () => {
  const { track } = await signal();
  vi.stubGlobal(
    'AudioContext',
    vi.fn(function () {
      throw new Error('Unavailable');
    })
  );
  const meters = levels();
  meters.update([['local', { mediaStreamTrack: track }]]);
  expect(meters.get('local')).toBeUndefined();
  expect(track.readyState).toBe('live');
});

it('releases a suspended context even before its resume finishes', () => {
  let rejectResume: (reason: Error) => void = () => {};
  const resume = new Promise<void>((_resolve, reject) => {
    rejectResume = reject;
  });
  const close = vi.fn(async () => {
    rejectResume(new Error('Closed before resume'));
  });
  const create = vi.fn(function () {
    return { state: 'suspended', resume: () => resume, close };
  });
  vi.stubGlobal('AudioContext', create);
  const meters = levels();
  const stream = { enabled: true, muted: false, readyState: 'live' } as MediaStreamTrack;
  meters.update([['local', { mediaStreamTrack: stream }]]);
  meters.update([['local', { mediaStreamTrack: stream }]]);
  expect(create).toHaveBeenCalledOnce();
  expect(meters.get('local')).toBeUndefined();
  meters.clear();
  expect(close).toHaveBeenCalledOnce();
});
