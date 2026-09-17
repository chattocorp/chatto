/** Non-reactive, pre-volume meters for existing call tracks; never captures or plays audio. */
export class TrackAudioLevels {
  private meters = new Map<
    string,
    {
      track: MediaStreamTrack;
      source: MediaStreamAudioSourceNode;
      analyser: AnalyserNode;
      samples: Float32Array<ArrayBuffer>;
      level: number;
    }
  >();

  /** Reconcile track replacements and removals using the call-owned audio context. */
  sync(context: AudioContext | null, entries: Iterable<readonly [string, MediaStreamTrack]>) {
    const tracks = new Map(entries);
    for (const [identity, meter] of this.meters) {
      if (!context || tracks.get(identity) !== meter.track) {
        meter.source.disconnect();
        meter.analyser.disconnect();
        this.meters.delete(identity);
      }
    }
    if (!context) return;
    for (const [identity, track] of tracks) {
      if (this.meters.has(identity) || track.readyState === 'ended') continue;
      let source: MediaStreamAudioSourceNode | undefined;
      try {
        source = context.createMediaStreamSource(new MediaStream([track]));
        const analyser = context.createAnalyser();
        analyser.fftSize = 1024;
        source.connect(analyser);
        this.meters.set(identity, {
          track,
          source,
          analyser,
          samples: new Float32Array(analyser.fftSize),
          level: 0
        });
      } catch {
        // Metering is optional; a missing/ended track must not interrupt a call.
        source?.disconnect();
      }
    }
  }

  /** Sample RMS amplitude before listener-local volume or mute is applied. */
  sample() {
    for (const meter of this.meters.values()) {
      if (meter.track.muted || !meter.track.enabled || meter.track.readyState === 'ended') {
        meter.level = 0;
        continue;
      }
      meter.analyser.getFloatTimeDomainData(meter.samples);
      let sum = 0;
      for (const value of meter.samples) sum += value * value;
      meter.level = Math.min(1, Math.sqrt(sum / meter.samples.length));
    }
  }

  get(identity: string): number {
    return this.meters.get(identity)?.level ?? 0;
  }

  /** Distinguish a working meter from missing or unsupported capture. */
  has(identity: string): boolean {
    return this.meters.has(identity);
  }

  /** Disconnect only meter nodes; the call still owns its context and tracks. */
  clear() {
    this.sync(null, new Map());
  }
}
