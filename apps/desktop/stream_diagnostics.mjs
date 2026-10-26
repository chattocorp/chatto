/** An in-memory, bounded report of native screen-share performance.
 * SDK objects and free-form log messages must never enter this report.
 */
export class StreamDiagnostics {
  #entries = [];
  #started = performance.now();

  recordLifecycle(phase, codec, simulcast = true, tunedH264 = false) {
    if (!['start', 'started', 'stop', 'ended', 'failed'].includes(phase)) return;
    this.#append({
      phase,
      requestedCodec: codec === 'h264' ? 'h264' : 'h265',
      tunedH264: tunedH264 === true,
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
        note: 'Publisher samples only: every second for the first 20 seconds, then every 5 seconds. Rates are bits/s; delays are ms. limitsBps lists configured video layer ceilings per record. availableOutgoingBps is the shared publisher transport estimate. targetBps is instantaneous; actualBps and encodedTargetBps use intervalMs. encodedTargetBps is the hypothetical bitrate of encoded frames at their target size, not a bandwidth estimate. encodedFps and sentFps use that interval; captureFps is the SDK source FPS. packetSendDelayMs is average packet send delay in that interval. Missing fields are unavailable. QP is codec-specific. Counters are cumulative.'
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
    sdkVersion:
      typeof value.sdkVersion === 'string' && /^\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(value.sdkVersion)
        ? value.sdkVersion
        : undefined,
    ...layerConfiguration(value.simulcast ?? true),
    maintainsResolution: boolean(value.maintainsResolution),
    tunedH264: boolean(value.tunedH264),
    tunedEncoder: parseTunedEncoder(value.tunedEncoder),
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
        encoder: known(layer.encoder, [
          'VideoToolbox',
          'ChattoVideoToolboxH264',
          'libaom',
          'libvpx',
          'OpenH264'
        ]),
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
        'intervalMs',
        'encodedTargetBps',
        'encodedFps',
        'sentFps',
        'captureFps',
        'packetSendDelayMs',
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

/** VT status codes can be negative; other counters and rates cannot. */
function parseTunedEncoder(value) {
  if (!value || typeof value !== 'object') return undefined;
  const result = {};
  if (['average', 'vbr'].includes(value.rateControl)) result.rateControl = value.rateControl;
  if (['auto', 'average', 'vbr'].includes(value.requestedRateControl))
    result.requestedRateControl = value.requestedRateControl;
  for (const key of [
    'requestedBps',
    'averageBps',
    'ceilingBps',
    'appliedAverageBps',
    'appliedCeilingBps',
    'vbvBufferMs',
    'framesEncoded',
    'framesDropped',
    'queueDrops',
    'peakPendingFrames',
    'callbackRejected'
  ]) {
    if (Number.isSafeInteger(value[key]) && value[key] >= 0) result[key] = value[key];
  }
  if (typeof value.hardware === 'boolean') result.hardware = value.hardware;
  if (typeof value.profile === 'string' && /^[0-9a-f]{6}$/i.test(value.profile))
    result.profile = value.profile;
  for (const key of ['lastStatus', 'fallbackStatus']) {
    if (Number.isInteger(value[key]) && value[key] >= -2147483648 && value[key] <= 2147483647)
      result[key] = value[key];
  }
  if (
    [
      'idle',
      'create',
      'prepare',
      'hardware',
      'encode',
      'output',
      'packetize',
      'readAverageBitRate',
      'RealTime',
      'ProfileLevel',
      'AllowFrameReordering',
      'ExpectedFrameRate',
      'MaxKeyFrameInterval',
      'MaxKeyFrameIntervalDuration',
      'PrioritizeEncodingSpeedOverQuality',
      'MaxAllowedFrameQP',
      'DataRateLimits',
      'AverageBitRate',
      'VariableBitRate',
      'VBVMaxBitRate',
      'VBVBufferDuration',
      'VBVInitialDelayPercentage'
    ].includes(value.operation)
  )
    result.operation = value.operation;
  return result;
}

/** Include limits with each record because one report can contain several share modes. */
function layerConfiguration(simulcast) {
  return { simulcast, limitsBps: simulcast ? [1_000_000, 4_000_000, 8_000_000] : [8_000_000] };
}
