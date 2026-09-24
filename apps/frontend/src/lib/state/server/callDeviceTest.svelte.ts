import { normalizeMicrophoneEffects, type MicrophoneEffects } from '$lib/audio/microphoneEffects';
import { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { GATE_OFF, microphoneMeter } from '$lib/audio/noiseGate';
import type { Track } from 'livekit-client';

const MAX_RECORDING_MS = 10_000;

type SelectableAudioContext = AudioContext & {
  setSinkId?: (deviceId: string) => Promise<void>;
};

/** An explicitly started, page-owned microphone test. The recording stays in browser memory. */
export class CallDeviceTest {
  active = $state(false);
  pending = $state(false);
  finalizing = $state(false);
  playing = $state(false);
  playbackPending = $state(false);
  hasRecording = $state(false);
  playbackBlocked = $state(false);
  level = $state(0);
  error = $state(false);
  gateUnavailable = $state(false);
  #processor: MicrophoneProcessor | null = null;
  #generation = 0;
  #stream: MediaStream | null = null;
  #context: AudioContext | null = null;
  #playbackContext: SelectableAudioContext | null = null;
  #audio: HTMLAudioElement | null = null;
  #recordingUrl: string | null = null;
  #recorder: MediaRecorder | null = null;
  #chunks: Blob[] = [];
  #monitor: MediaStreamAudioSourceNode | null = null;
  #frame = 0;
  #timeout: ReturnType<typeof setTimeout> | null = null;
  #outputChange: Promise<unknown> = Promise.resolve();
  #speakerId = '';

  async start(
    deviceId: string,
    speakerId = '',
    threshold: () => number = () => -60,
    effects: () => MicrophoneEffects = normalizeMicrophoneEffects
  ): Promise<void> {
    this.cancel();
    const generation = this.#generation;
    this.pending = true;
    this.error = false;
    this.#speakerId = speakerId;
    const needsProcessing = () => {
      const value = effects();
      return (
        threshold() > GATE_OFF ||
        value.lowCut ||
        value.equalizer ||
        value.compressor ||
        (value.polish ?? 0) > 0
      );
    };
    const processing = needsProcessing();
    try {
      if (typeof MediaRecorder === 'undefined') throw new Error('Recording unavailable');
      const audio = new Audio();
      this.#audio = audio;
      audio.onended = () => {
        if (generation === this.#generation) this.playing = false;
      };
      if (speakerId) await this.#selectOutput(speakerId);
      if (generation !== this.#generation) return;
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
          channelCount: { ideal: 1 },
          // The test records the same signal the call would send. Playback starts
          // only after capture ends, so it cannot feed into this microphone.
          echoCancellation: false,
          noiseSuppression: processing,
          autoGainControl: false
        },
        video: false
      });
      if (generation !== this.#generation) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      this.#stream = stream;
      stream.getAudioTracks().forEach((track) =>
        track.addEventListener(
          'ended',
          () => {
            if (generation === this.#generation && (this.active || this.pending)) {
              this.cancel();
              this.error = true;
            }
          },
          { once: true }
        )
      );
      const context = new AudioContext();
      this.#context = context;
      let processor: MicrophoneProcessor | undefined;
      let readLevel: () => number;
      let recordedStream = stream;
      if (processing) {
        processor = new MicrophoneProcessor(threshold());
        this.#processor = processor;
        processor.setEffects(effects());
        await processor.init({
          track: stream.getAudioTracks()[0],
          audioContext: context,
          kind: 'audio' as Track.Kind.Audio
        });
        if (generation !== this.#generation) return;
        this.gateUnavailable = processor.unavailable;
        recordedStream = new MediaStream([processor.processedTrack ?? stream.getAudioTracks()[0]]);
        readLevel = () => processor!.level;
      } else {
        const analyser = context.createAnalyser();
        this.#monitor = context.createMediaStreamSource(stream);
        this.#monitor.connect(analyser);
        const samples = new Float32Array(analyser.fftSize);
        readLevel = () => {
          analyser.getFloatTimeDomainData(samples);
          return Math.sqrt(samples.reduce((sum, value) => sum + value * value, 0) / samples.length);
        };
      }
      await context.resume();
      if (generation !== this.#generation) return;
      const recorder = new MediaRecorder(recordedStream);
      this.#recorder = recorder;
      recorder.ondataavailable = ({ data }) => {
        if (generation === this.#generation && data.size) this.#chunks.push(data);
      };
      recorder.onerror = () => {
        if (generation === this.#generation) {
          this.cancel();
          this.error = true;
        }
      };
      recorder.onstop = () => {
        recorder.ondataavailable = null;
        recorder.onerror = null;
        recorder.onstop = null;
        if (generation === this.#generation && this.finalizing)
          void this.#finishPlayback(generation);
      };
      recorder.start();
      this.active = true;
      this.#timeout = setTimeout(() => this.stop(), MAX_RECORDING_MS);
      const sample = () => {
        if (generation !== this.#generation || !this.active) return;
        if (processing !== needsProcessing()) {
          void this.start(deviceId, this.#speakerId, threshold, effects);
          return;
        }
        processor?.setThreshold(threshold());
        processor?.setEffects(effects());
        this.gateUnavailable = processor?.unavailable ?? false;
        this.level = microphoneMeter(readLevel());
        this.#frame = requestAnimationFrame(sample);
      };
      sample();
    } catch {
      if (generation === this.#generation) {
        this.cancel();
        this.error = true;
      }
    } finally {
      if (generation === this.#generation) this.pending = false;
    }
  }

  /** Stop recording, release the microphone, and replay the completed sample. */
  stop(): void {
    if (this.pending) {
      this.cancel();
      return;
    }
    if (!this.active) return;
    this.active = false;
    this.finalizing = true;
    const recorder = this.#recorder;
    this.#recorder = null;
    try {
      if (recorder?.state !== 'recording') throw new Error('Recording stopped');
      recorder.stop();
    } catch {
      this.cancel();
      this.error = true;
      return;
    }
    this.#releaseCapture();
  }

  async #finishPlayback(generation: number): Promise<void> {
    const chunks = this.#chunks;
    this.#chunks = [];
    if (!chunks.length || chunks.every((chunk) => chunk.size === 0)) {
      this.finalizing = false;
      this.error = true;
      return;
    }
    try {
      const blob = new Blob(chunks, { type: chunks[0].type });
      this.#recordingUrl = URL.createObjectURL(blob);
      this.#audio!.src = this.#recordingUrl;
      this.hasRecording = true;
      await this.playAgain(generation);
    } catch {
      if (generation === this.#generation) {
        this.cancel();
        this.error = true;
      }
    }
  }

  /** Replay the sample after capture has ended; retain it if autoplay is blocked. */
  async playAgain(generation = this.#generation): Promise<void> {
    if (
      generation !== this.#generation ||
      !this.hasRecording ||
      !this.#audio ||
      this.playbackPending
    )
      return;
    this.playbackPending = true;
    this.playbackBlocked = false;
    try {
      await this.#outputChange;
      if (generation !== this.#generation || !this.#audio) return;
      await this.#playbackContext?.resume();
      if (generation !== this.#generation || !this.#audio) return;
      this.#audio.currentTime = 0;
      await this.#audio.play();
      if (generation !== this.#generation) return;
      this.playing = true;
    } catch {
      if (generation === this.#generation) this.playbackBlocked = true;
    } finally {
      if (generation === this.#generation) {
        this.playbackPending = false;
        this.finalizing = false;
      }
    }
  }

  /** Stop sample playback without discarding the sample. */
  stopPlayback(): void {
    this.#audio?.pause();
    this.playing = false;
  }

  /** Switch replay output without reopening the microphone. */
  setSpeaker(speakerId: string): Promise<boolean> {
    const generation = this.#generation;
    const change = this.#outputChange.then(async () => {
      if (generation !== this.#generation || !this.#audio) return false;
      try {
        await this.#selectOutput(speakerId);
        if (generation !== this.#generation) return false;
        this.#speakerId = speakerId;
        return true;
      } catch {
        if (generation === this.#generation) {
          this.cancel();
          this.error = true;
        }
        return false;
      }
    });
    this.#outputChange = change;
    return change;
  }

  /** Route only replay audio; context-only sinks use a separate playback graph. */
  async #selectOutput(speakerId: string): Promise<void> {
    const audio = this.#audio!;
    if (typeof audio.setSinkId === 'function') {
      await audio.setSinkId(speakerId);
      return;
    }
    if (!speakerId && !this.#playbackContext) return;
    let context = this.#playbackContext;
    if (!context) {
      context = new AudioContext() as SelectableAudioContext;
      if (typeof context.setSinkId !== 'function') {
        void context.close().catch(() => {});
        throw new Error('Output selection unavailable');
      }
      this.#playbackContext = context;
      context.createMediaElementSource(audio).connect(context.destination);
      await context.resume();
    }
    await context.setSinkId!(speakerId);
  }

  #releaseCapture(): void {
    if (this.#timeout) clearTimeout(this.#timeout);
    this.#timeout = null;
    cancelAnimationFrame(this.#frame);
    this.#monitor?.disconnect();
    this.#monitor = null;
    this.#processor?.dispose();
    this.#processor = null;
    this.#stream?.getTracks().forEach((track) => track.stop());
    this.#stream = null;
    void this.#context?.close().catch(() => {});
    this.#context = null;
    this.level = 0;
    this.gateUnavailable = false;
  }

  /** Cancel pending capture, recording, and playback on navigation or restart. */
  cancel(): void {
    this.#generation++;
    this.#outputChange = Promise.resolve();
    if (this.#recorder) {
      this.#recorder.ondataavailable = null;
      this.#recorder.onerror = null;
      this.#recorder.onstop = null;
      try {
        if (this.#recorder.state === 'recording') this.#recorder.stop();
      } catch {
        // Capture must still be released if the recorder rejects stop.
      }
    }
    this.#recorder = null;
    this.#chunks = [];
    this.#releaseCapture();
    this.#audio?.pause();
    if (this.#audio) {
      this.#audio.onended = null;
      this.#audio.removeAttribute('src');
    }
    this.#audio = null;
    void this.#playbackContext?.close().catch(() => {});
    this.#playbackContext = null;
    if (this.#recordingUrl) URL.revokeObjectURL(this.#recordingUrl);
    this.#recordingUrl = null;
    this.active = false;
    this.pending = false;
    this.finalizing = false;
    this.playing = false;
    this.playbackPending = false;
    this.hasRecording = false;
    this.playbackBlocked = false;
  }
}
