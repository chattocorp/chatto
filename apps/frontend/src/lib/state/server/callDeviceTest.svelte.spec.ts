import { userEvent } from 'vitest/browser';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CallDeviceTest } from './callDeviceTest.svelte';

describe('CallDeviceTest', () => {
  afterEach(() => vi.restoreAllMocks());

  it('meters audio, records a playable clip, and releases capture on completion', async () => {
    const source = new AudioContext();
    const oscillator = source.createOscillator();
    const destination = source.createMediaStreamDestination();
    oscillator.connect(destination);
    oscillator.start();
    const button = document.createElement('button');
    button.textContent = 'Activate test audio';
    document.body.append(button);
    button.onclick = () => {
      void source.resume();
    };
    await userEvent.click(button);
    button.remove();
    const stream = destination.stream;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const test = new CallDeviceTest();
    try {
      await test.start('');
      expect(test.active).toBe(true);
      await vi.waitFor(() => expect(test.level).toBeGreaterThan(0));
      test.record();
      expect(test.recording).toBe(true);
      test.finishRecording();
      await vi.waitFor(() => expect(test.clipURL).toMatch(/^blob:/));
      expect((await (await fetch(test.clipURL)).blob()).size).toBeGreaterThan(0);
      expect(stream.getTracks().every((track) => track.readyState === 'ended')).toBe(true);
      expect(test.active).toBe(false);
      test.stop();
      expect(test.clipURL).toBe('');
    } finally {
      test.stop();
      oscillator.stop();
      await source.close();
    }
  });

  it('does not request capture until explicitly started', () => {
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    const test = new CallDeviceTest();
    expect(test.active).toBe(false);
    expect(capture).not.toHaveBeenCalled();
    test.stop();
  });

  it('stops a stream that arrives after cancellation', async () => {
    let resolve!: (stream: MediaStream) => void;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const stop = vi.fn();
    const test = new CallDeviceTest();
    const pending = test.start('mic');
    test.stop();
    resolve({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    await pending;
    expect(stop).toHaveBeenCalledOnce();
    expect(test.active).toBe(false);
    expect(test.pending).toBe(false);
  });

  it('handles denial without leaving active capture state', async () => {
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockRejectedValue(
      new DOMException('Denied', 'NotAllowedError')
    );
    const test = new CallDeviceTest();
    await test.start('mic');
    expect(test.error).toBe(true);
    expect(test.pending).toBe(false);
    expect(test.active).toBe(false);
  });

  it('does not let a rejected older request replace a newer test state', async () => {
    let reject!: (reason: unknown) => void;
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    capture.mockImplementationOnce(
      () =>
        new Promise((_, fail) => {
          reject = fail;
        })
    );
    capture.mockImplementationOnce(() => new Promise(() => {}));
    const test = new CallDeviceTest();
    const older = test.start('old');
    void test.start('new');
    reject(new Error('old failure'));
    await older;
    expect(test.pending).toBe(true);
    expect(test.error).toBe(false);
    test.stop();
  });
});
