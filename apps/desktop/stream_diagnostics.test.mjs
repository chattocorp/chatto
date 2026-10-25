import assert from 'node:assert/strict';
import test from 'node:test';
import { parseStreamDiagnostic, StreamDiagnostics } from './stream_diagnostics.mjs';

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
