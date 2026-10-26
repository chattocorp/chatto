import assert from 'node:assert/strict';
import test from 'node:test';
import { parseStreamDiagnostic, StreamDiagnostics } from './stream_diagnostics.mjs';

test('custom encoder reports preserve rate changes and errors without copying private text', () => {
  const value = {
    protocolVersion: 1,
    kind: 'diagnostic',
    phase: 'sample',
    requestedCodec: 'h264',
    simulcast: false,
    tunedH264: true,
    layers: [{ encoder: 'ChattoVideoToolboxH264' }],
    tunedEncoder: {
      rateControl: 'vbr',
      appliedCeilingBps: 3_000_000,
      vbvBufferMs: 250,
      requestedBps: 3_000_000,
      averageBps: 2_250_000,
      ceilingBps: 3_000_000,
      appliedAverageBps: 2_250_000,
      hardware: true,
      profile: '640034',
      framesEncoded: 100,
      framesDropped: 1,
      callbackRejected: 0,
      lastStatus: -12900,
      operation: 'prepare',
      token: 'private token'
    }
  };
  const log = new StreamDiagnostics();
  log.recordLifecycle('start', 'h264', false, true);
  log.recordNative(value);
  const [, start, sample] = log.report({ version: 'test' }).split('\n').map(JSON.parse);
  assert.equal(start.tunedH264, true);
  assert.equal(sample.tunedH264, true);
  assert.equal(sample.layers[0].encoder, 'ChattoVideoToolboxH264');
  assert.equal(sample.tunedEncoder.lastStatus, -12900);
  assert.equal(sample.tunedEncoder.appliedAverageBps, 2_250_000);
  assert.equal(sample.tunedEncoder.rateControl, 'vbr');
  assert.equal(sample.tunedEncoder.appliedCeilingBps, 3_000_000);
  assert.equal(sample.tunedEncoder.vbvBufferMs, 250);
  assert.ok(!JSON.stringify(sample).includes('private'));
  for (const invalid of ['private value', NaN, Infinity, -1, 2 ** 54]) {
    const tunedEncoder = Object.fromEntries(
      Object.keys(value.tunedEncoder).map((key) => [key, invalid])
    );
    // A negative integer is a valid OSStatus, but not a valid rate/counter.
    const parsed = parseStreamDiagnostic({ ...value, tunedEncoder }).tunedEncoder;
    assert.deepEqual(parsed, invalid === -1 ? { lastStatus: -1 } : {});
  }
});

test('diagnostics strip identifiers, credentials, addresses, SDP, and arbitrary text', () => {
  const value = {
    protocolVersion: 1,
    kind: 'diagnostic',
    phase: 'sample',
    requestedCodec: 'h265',
    availableOutgoingBps: 9_000_000,
    candidatePair: { address: 'private transport', id: 'private pair' },
    token: 'secret',
    address: '192.0.2.10',
    window: 'private window',
    layers: [
      {
        codec: 'video/h265',
        encoder: 'unexpected private data',
        sdp: 'secret SDP',
        trackId: 'private track',
        width: 1920,
        actualBps: 7_500_000,
        limitation: 'bandwidth'
      }
    ]
  };
  const parsed = parseStreamDiagnostic(value);
  const text = JSON.stringify(parsed);
  for (const secret of ['secret', '192.0.2.10', 'private']) assert.ok(!text.includes(secret));
  assert.equal(parsed.layers[0].encoder, 'unknown');
  assert.equal(parsed.layers[0].actualBps, 7_500_000);
  assert.equal(parsed.layers[0].limitation, 'bandwidth');
  assert.equal(parsed.availableOutgoingBps, 9_000_000);
  assert.throws(() => parseStreamDiagnostic({ ...value, layers: Array(13).fill({}) }));
  assert.throws(() => parseStreamDiagnostic({ ...value, requestedCodec: 'private value' }));
});

test('mixed share reports keep the layer mode and limits with each record', () => {
  const log = new StreamDiagnostics();
  log.recordLifecycle('start', 'h265');
  log.recordLifecycle('start', 'h264', false);
  const value = {
    protocolVersion: 1,
    kind: 'diagnostic',
    phase: 'sample',
    requestedCodec: 'h264',
    simulcast: false,
    maintainsResolution: true,
    availableOutgoingBps: 6_000_000,
    layers: []
  };
  log.recordNative(value);
  const [, hevc, start, sample] = log.report({ version: 'test' }).split('\n').map(JSON.parse);
  assert.equal(hevc.simulcast, true);
  assert.deepEqual(hevc.limitsBps, [1_000_000, 4_000_000, 8_000_000]);
  for (const record of [start, sample]) {
    assert.equal(record.requestedCodec, 'h264');
    assert.equal(record.simulcast, false);
    assert.deepEqual(record.limitsBps, [8_000_000]);
  }
  assert.equal(sample.availableOutgoingBps, 6_000_000);
  assert.equal(sample.maintainsResolution, true);
  for (const preference of [false, undefined]) {
    assert.equal(
      parseStreamDiagnostic({ ...value, maintainsResolution: preference }).maintainsResolution,
      preference
    );
  }
  assert.equal(
    parseStreamDiagnostic({ ...value, maintainsResolution: 'private value' }).maintainsResolution,
    undefined
  );
  for (const invalid of [-1, Infinity, NaN, 'private value']) {
    assert.equal(
      parseStreamDiagnostic({ ...value, availableOutgoingBps: invalid }).availableOutgoingBps,
      undefined
    );
  }
  assert.throws(() => parseStreamDiagnostic({ ...value, simulcast: 'false' }));
});

test('automatic fallback diagnostics retain only allowlisted modes and numeric status', () => {
  const sample = {
    protocolVersion: 1,
    kind: 'diagnostic',
    phase: 'sample',
    requestedCodec: 'h264',
    layers: [],
    tunedEncoder: {
      requestedRateControl: 'auto',
      rateControl: 'average',
      fallbackStatus: -12900,
      lastStatus: 0
    }
  };
  assert.deepEqual(parseStreamDiagnostic(sample).tunedEncoder, sample.tunedEncoder);
  sample.tunedEncoder.requestedRateControl = 'private value';
  sample.tunedEncoder.fallbackStatus = 'private value';
  assert.equal(parseStreamDiagnostic(sample).tunedEncoder.requestedRateControl, undefined);
  assert.equal(parseStreamDiagnostic(sample).tunedEncoder.fallbackStatus, undefined);
});

test('reports retain only the most recent 60 records and omit invalid numbers', () => {
  const log = new StreamDiagnostics();
  log.recordLifecycle('start', 'h265');
  for (let i = 0; i < 65; i++)
    log.recordNative({
      protocolVersion: 1,
      kind: 'diagnostic',
      phase: 'sample',
      requestedCodec: 'h265',
      layers: [{ width: i, fps: NaN, actualBps: -1 }]
    });
  const lines = log.report({ version: 'test' }).split('\n').map(JSON.parse);
  assert.equal(lines.length, 61);
  assert.equal(lines[1].layers[0].width, 5);
  assert.equal(lines.at(-1).layers[0].width, 64);
  assert.equal(lines[1].layers[0].fps, undefined);
  assert.equal(lines[1].layers[0].actualBps, undefined);
});

test('interval diagnostics retain numeric metrics and reject private or invalid values', () => {
  const metrics = {
    intervalMs: 2000,
    encodedTargetBps: 8_000_000,
    encodedFps: 55,
    sentFps: 54,
    captureFps: 60,
    packetSendDelayMs: 2
  };
  const value = {
    protocolVersion: 1,
    kind: 'diagnostic',
    phase: 'sample',
    requestedCodec: 'h264',
    sdkVersion: '2.18.0',
    layers: [metrics]
  };
  const parsed = parseStreamDiagnostic(value);
  assert.equal(parsed.sdkVersion, '2.18.0');
  for (const [key, number] of Object.entries(metrics)) assert.equal(parsed.layers[0][key], number);
  for (const invalid of [-1, NaN, Infinity, 'private value', undefined]) {
    const layers = [Object.fromEntries(Object.keys(metrics).map((key) => [key, invalid]))];
    const result = parseStreamDiagnostic({ ...value, sdkVersion: 'private value', layers });
    assert.equal(result.sdkVersion, undefined);
    for (const key of Object.keys(metrics)) assert.equal(result.layers[0][key], undefined);
  }
});
