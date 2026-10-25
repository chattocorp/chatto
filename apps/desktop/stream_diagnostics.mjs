/** An in-memory, bounded report of native screen-share performance.
 * SDK objects and free-form log messages must never enter this report.
 */
export class StreamDiagnostics {
  #entries = [];
  #started = performance.now();

  recordLifecycle(phase, codec, simulcast = true) {
    if (!['start', 'started', 'stop', 'ended', 'failed'].includes(phase)) return;
    this.#append({
      phase,
      requestedCodec: codec === 'h264' ? 'h264' : 'h265',
      ...layerConfiguration(simulcast)
    });
  }

  recordNative(value) {
    return this.#append(parseStreamDiagnostic(value));
  }

  #append(entry) {
    const record = { elapsedMs: Math.round(performance.now() - this.#started), ...entry };
    this.#entries.push(record);
    if (this.#entries.length > 60) this.#entries.shift();
    return record;
  }

  report({ version, electron, platform, arch, variant }) {
    return [
      JSON.stringify({
        report: 'chatto-screen-share-v1',
        version,
        electron,
        platform,
        arch,
        variant,
        note: 'Publisher samples only. Rates are bits/s; delays are ms. limitsBps lists configured video layer ceilings per record. availableOutgoingBps is the shared publisher transport estimate, not a per-layer limit. Missing fields are unavailable. QP is codec-specific. Counters are cumulative.'
      }),
      ...this.#entries.map((entry) => JSON.stringify(entry))
    ].join('\n');
  }
}

/** Reconstruct an allowlisted record; never copy arbitrary helper JSON or SDP. */
export function parseStreamDiagnostic(value) {
  if (
    value?.protocolVersion !== 1 ||
    value?.kind !== 'diagnostic' ||
    !['connecting', 'sample', 'fallback'].includes(value.phase) ||
    !['h264', 'h265'].includes(value.requestedCodec) ||
    (value.simulcast !== undefined && typeof value.simulcast !== 'boolean') ||
    !Array.isArray(value.layers) ||
    value.layers.length > 12
  ) {
    throw new Error('Invalid screen-share diagnostic.');
  }
  const number = (input) =>
    typeof input === 'number' && Number.isFinite(input) && input >= 0 ? input : undefined;
  const boolean = (input) => (typeof input === 'boolean' ? input : undefined);
  const known = (input, values) => (values.includes(input) ? input : 'unknown');
  return {
    phase: value.phase,
    requestedCodec: value.requestedCodec,
    ...layerConfiguration(value.simulcast ?? true),
    availableOutgoingBps: number(value.availableOutgoingBps),
    captureWidth: number(value.captureWidth),
    captureHeight: number(value.captureHeight),
    layers: value.layers.map((layer) => {
      if (!layer || typeof layer !== 'object') throw new Error('Invalid screen-share layer.');
      const result = {
        codec: known(layer.codec, [
          'video/h264',
          'video/h265',
          'video/vp8',
          'video/vp9',
          'video/av1'
        ]),
        profile:
          typeof layer.profile === 'string' && /^[0-9a-f]{1,8}$/i.test(layer.profile)
            ? layer.profile
            : undefined,
        encoder: known(layer.encoder, ['VideoToolbox', 'libaom', 'libvpx', 'OpenH264']),
        limitation: known(layer.limitation, ['none', 'cpu', 'bandwidth', 'other']),
        active: boolean(layer.active),
        powerEfficient: boolean(layer.powerEfficient)
      };
      for (const key of [
        'width',
        'height',
        'fps',
        'targetBps',
        'actualBps',
        'meanQP',
        'encodeMsPerFrame',
        'packetsLost',
        'roundTripMs',
        'retransmittedBytes',
        'keyFrames',
        'nackCount',
        'pliCount'
      ])
        result[key] = number(layer[key]);
      return result;
    })
  };
}

/** Include limits with each record because one report can contain several share modes. */
function layerConfiguration(simulcast) {
  return { simulcast, limitsBps: simulcast ? [1_000_000, 4_000_000, 8_000_000] : [8_000_000] };
}
