/** Browser-local call meters. Analysis never replaces, plays, or stops a media track. */

/** The media surfaces exposed by LiveKit audio tracks. */
export interface AudioLevelTrack {
  mediaStreamTrack?: MediaStreamTrack;
  /** Local tracks retain their original capture here, before optional processing. */
  mediaStream?: MediaStream;
}

/** Poll current audio often enough to show activity within one tenth of a second. */
export const AUDIO_LEVEL_SAMPLE_INTERVAL_MS = 30;

/**
 * Own one analysis-only context for microphone and decoded remote audio.
 * Update with the current tracks; clear on backgrounding and call disposal.
 * A failed or suspended meter returns undefined for the SDK-level fallback.
 */
export class CallAudioLevels {
  #context?: AudioContext;
  #unavailable = false;
  #levels = new Map<string, number>();
  #meters = new Map<
    string,
    {
      track: MediaStreamTrack;
      source: MediaStreamAudioSourceNode;
      analyser: AnalyserNode;
      samples: Float32Array<ArrayBuffer>;
    }
  >();

  /** Reconcile replacements and removals, then sample all current audio tracks. */
  update(entries: Iterable<readonly [string, AudioLevelTrack]>): void {
    if (document.hidden) {
      this.clear();
      return;
    }
    const tracks = new Map(entries);
    let needsAnalysis = false;
    this.#levels.clear();
    for (const identity of this.#meters.keys()) {
      if (!tracks.has(identity)) this.#remove(identity);
    }
    for (const [identity, audio] of tracks) {
      const track = audio.mediaStream?.getAudioTracks()[0] ?? audio.mediaStreamTrack;
      if (!track) {
        this.#remove(identity);
        continue;
      }
      if (!track.enabled || track.muted || track.readyState !== 'live') {
        this.#remove(identity);
        this.#levels.set(identity, 0);
        continue;
      }
      needsAnalysis = true;
      let meter = this.#meters.get(identity);
      if (meter?.track !== track) {
        this.#remove(identity);
        meter = undefined;
      }
      try {
        if (!this.#context && !this.#unavailable) {
          this.#context = new AudioContext();
          void this.#context.resume().catch(() => {});
        }
        const context = this.#context;
        if (!context || context.state !== 'running') continue;
        if (!meter) {
          const analyser = context.createAnalyser();
          analyser.fftSize = 1024;
          const source = context.createMediaStreamSource(new MediaStream([track]));
          meter = { track, source, analyser, samples: new Float32Array(analyser.fftSize) };
          this.#meters.set(identity, meter);
          source.connect(analyser);
        }
        meter.analyser.getFloatTimeDomainData(meter.samples);
        let energy = 0;
        for (const sample of meter.samples) energy += sample * sample;
        this.#levels.set(identity, Math.min(1, Math.sqrt(energy / meter.samples.length)));
      } catch {
        // Visual feedback must never prevent a call from sending or playing audio.
        this.#unavailable = true;
        this.#remove(identity);
      }
    }
    if (!needsAnalysis) this.#closeContext();
  }

  /** Undefined means no local measurement is available; zero is measured silence. */
  get(identity: string): number | undefined {
    return this.#levels.get(identity);
  }

  #remove(identity: string): void {
    const meter = this.#meters.get(identity);
    meter?.source.disconnect();
    meter?.analyser.disconnect();
    this.#meters.delete(identity);
  }

  #closeContext(): void {
    const context = this.#context;
    this.#context = undefined;
    if (context) void context.close().catch(() => {});
  }

  /** Release only analysis resources. Capture and playback keep their original tracks. */
  clear(): void {
    for (const identity of this.#meters.keys()) this.#remove(identity);
    this.#levels.clear();
    this.#closeContext();
    this.#unavailable = false;
  }
}
