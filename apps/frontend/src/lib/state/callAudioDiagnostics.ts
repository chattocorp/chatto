import type { Participant, RemoteTrackPublication, Room, RoomEventCallbacks } from 'livekit-client';

let nextCall = 0;
const SAMPLE_INTERVAL_MS = 10_000;

/**
 * Console-only remote audio diagnostics. Each call uses fresh numeric aliases.
 * Never pass SDK objects, error text, SDP, or raw stats to the console: they can
 * contain identities, credentials, device identifiers, and network addresses.
 */
export class CallAudioDiagnostics {
  private readonly call = ++nextCall;
  private readonly started = performance.now();
  private readonly participants = new Map<string, number>();
  private readonly sessions = new Map<string, number>();
  private readonly publications = new Map<string, number>();
  private readonly rtpStreams = new Map<string, number>();
  private readonly receivers = new WeakMap<RTCRtpReceiver, number>();
  private nextReceiver = 0;
  private readonly pending = new WeakSet<object>();
  private readonly listeners: (() => void)[] = [];
  private readonly errors = new Map<string, { count: number; lastLogged: number }>();
  private readonly timer: ReturnType<typeof setInterval>;
  private disposed = false;

  constructor(
    private readonly room: Room,
    sdkVersion?: string
  ) {
    this.listen('participantConnected', (participant) => {
      this.write('participant-connected', this.participantFields(participant));
    });
    this.listen('participantDisconnected', (participant) => {
      this.write('participant-disconnected', this.participantFields(participant));
    });
    for (const event of ['trackPublished', 'trackUnpublished'] as const) {
      this.listen(event, (publication, participant) =>
        this.trackEvent(event, publication, participant)
      );
    }
    for (const event of ['trackSubscribed', 'trackUnsubscribed'] as const) {
      this.listen(event, (_track, publication, participant) => {
        this.trackEvent(event, publication, participant);
      });
    }
    for (const event of [
      'trackSubscriptionStatusChanged',
      'trackSubscriptionPermissionChanged',
      'trackStreamStateChanged'
    ] as const) {
      this.listen(
        event,
        (publication: RemoteTrackPublication, _status: unknown, participant: Participant) => {
          this.trackEvent(event, publication, participant);
        }
      );
    }
    this.listen('trackSubscriptionFailed', (sid, participant, reason) => {
      this.failure('subscription-failed', {
        ...this.participantFields(participant),
        publication: this.alias(this.publications, sid),
        reason: finiteNumber(reason)
      });
    });
    this.listen('encryptionError', (error, participant) => {
      // The reason is numeric in CryptorError. Never log its message or identity.
      const reason = 'reason' in error ? finiteNumber(error.reason) : undefined;
      this.failure('encryption-error', {
        ...(participant ? this.participantFields(participant) : {}),
        reason: reason === 0 ? 'invalid-key' : reason === 1 ? 'missing-key' : 'internal-or-unknown'
      });
    });
    this.listen('audioPlaybackChanged', (playing) => this.write('playback', { playing }));
    for (const event of [
      'connected',
      'reconnecting',
      'signalReconnecting',
      'reconnected',
      'disconnected'
    ] as const) {
      this.listen(event, () => {
        this.write(event, {});
        this.sample();
      });
    }
    this.write('started', { sdkVersion });
    this.timer = setInterval(() => this.sample(), SAMPLE_INTERVAL_MS);
  }

  /** Stop collection before the call releases its tracks; ignore late stats results. */
  dispose(): void {
    if (this.disposed) return;
    this.write('stopped', {});
    this.disposed = true;
    clearInterval(this.timer);
    for (const remove of this.listeners) remove();
    this.listeners.length = 0;
    this.participants.clear();
    this.sessions.clear();
    this.publications.clear();
    this.rtpStreams.clear();
    this.errors.clear();
  }

  private listen<E extends keyof RoomEventCallbacks>(
    event: E,
    listener: RoomEventCallbacks[E]
  ): void {
    this.room.on(event, listener);
    this.listeners.push(() => this.room.off(event, listener));
  }

  private alias(map: Map<string, number>, key: string): number {
    let alias = map.get(key);
    if (alias === undefined) {
      alias = map.size + 1;
      map.set(key, alias);
    }
    return alias;
  }

  private participantFields(participant: Participant) {
    // Separate aliases let a report identify a rejoin without exposing either ID.
    return {
      participant: this.alias(this.participants, participant.identity),
      session: this.alias(this.sessions, participant.sid)
    };
  }

  private trackEvent(
    event: string,
    publication: RemoteTrackPublication,
    participant: Participant
  ): void {
    if (publication.kind !== 'audio') return;
    this.write(event, this.trackFields(publication, participant));
  }

  private trackFields(publication: RemoteTrackPublication, participant: Participant) {
    const track = publication.track;
    const receiver = track?.receiver;
    if (receiver && !this.receivers.has(receiver))
      this.receivers.set(receiver, ++this.nextReceiver);
    return {
      ...this.participantFields(participant),
      publication: this.alias(this.publications, publication.trackSid),
      receiver: receiver ? this.receivers.get(receiver) : undefined,
      microphone: publication.source === 'microphone',
      screenAudio: publication.source === 'screen_share_audio',
      subscribed: publication.isSubscribed,
      desired: publication.isDesired,
      allowed: publication.permissionStatus === 'allowed',
      enabled: publication.isEnabled,
      muted: publication.isMuted,
      hasTrack: !!track,
      streamActive: track?.streamState === 'active',
      mediaEnabled: track?.mediaStreamTrack.enabled,
      mediaMuted: track?.mediaStreamTrack.muted,
      mediaEnded: track?.mediaStreamTrack.readyState === 'ended',
      playbackAllowed: this.room.canPlaybackAudio,
      elements: track?.attachedElements.map((element) => ({
        paused: element.paused,
        muted: element.muted,
        volume: finiteNumber(element.volume),
        readyState: finiteNumber(element.readyState),
        errorCode: finiteNumber(element.error?.code),
        hasStream: !!element.srcObject,
        customOutput: !!element.sinkId && element.sinkId !== 'default'
      }))
    };
  }

  private sample(): void {
    if (this.disposed) return;
    for (const participant of this.room.remoteParticipants.values()) {
      for (const publication of participant.trackPublications.values()) {
        if (publication.kind !== 'audio') continue;
        this.write('audio-state', this.trackFields(publication, participant));
        void this.sampleStats(publication, participant);
      }
    }
  }

  private async sampleStats(
    publication: RemoteTrackPublication,
    participant: Participant
  ): Promise<void> {
    const track = publication.track;
    if (!track || this.pending.has(track)) return;
    this.pending.add(track);
    const current = () =>
      !this.disposed &&
      publication.track === track &&
      this.room.remoteParticipants.get(participant.identity) === participant &&
      participant.trackPublications.get(publication.trackSid) === publication;
    try {
      const report = await track.getRTCStatsReport();
      if (!current()) return;
      let found = false;
      report?.forEach((stats) => {
        if (stats.type !== 'inbound-rtp' || (stats.kind ?? stats.mediaType) !== 'audio') return;
        found = true;
        this.write('audio-rtp', {
          ...this.participantFields(participant),
          publication: this.alias(this.publications, publication.trackSid),
          receiver: track.receiver ? this.receivers.get(track.receiver) : undefined,
          rtpStream: this.alias(this.rtpStreams, stats.id),
          packetsReceived: finiteNumber(stats.packetsReceived),
          packetsLost: finiteNumber(stats.packetsLost),
          bytesReceived: finiteNumber(stats.bytesReceived),
          jitter: finiteNumber(stats.jitter),
          totalSamplesReceived: finiteNumber(stats.totalSamplesReceived),
          totalAudioEnergy: finiteNumber(stats.totalAudioEnergy),
          totalSamplesDuration: finiteNumber(stats.totalSamplesDuration),
          lastPacketAgeMs:
            finiteNumber(stats.timestamp) !== undefined &&
            finiteNumber(stats.lastPacketReceivedTimestamp) !== undefined
              ? finiteNumber(stats.timestamp - stats.lastPacketReceivedTimestamp)
              : undefined,
          concealedSamples: finiteNumber(stats.concealedSamples)
        });
      });
      if (!found) this.write('audio-rtp-unavailable', this.trackFields(publication, participant));
    } catch {
      if (current())
        this.failure('stats-failed', {
          ...this.participantFields(participant),
          publication: this.alias(this.publications, publication.trackSid)
        });
    } finally {
      this.pending.delete(track);
    }
  }

  /** Bound repeated SDK errors to one entry per category/track per sampling interval. */
  private failure(event: string, fields: Record<string, unknown>): void {
    const key = JSON.stringify([event, fields]);
    const now = performance.now();
    const previous = this.errors.get(key);
    const count = (previous?.count ?? 0) + 1;
    const log = !previous || now - previous.lastLogged >= SAMPLE_INTERVAL_MS;
    this.errors.set(key, { count, lastLogged: log ? now : previous.lastLogged });
    if (log) this.write(event, { ...fields, occurrences: count });
  }

  private write(event: string, fields: Record<string, unknown>): void {
    if (this.disposed) return;
    // Serialize now so developer tools cannot later expand a changed SDK object.
    console.info(
      '[call-audio]',
      JSON.stringify({
        call: this.call,
        elapsedMs: Math.round(performance.now() - this.started),
        event,
        ...fields
      })
    );
  }
}

function finiteNumber(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}
