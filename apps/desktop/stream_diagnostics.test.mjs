import assert from 'node:assert/strict';
import test from 'node:test';
import { parseStreamDiagnostic, StreamDiagnostics } from './stream_diagnostics.mjs';

test('diagnostics strip identifiers, credentials, addresses, SDP, and arbitrary text', () => {
  const value = {
    protocolVersion: 1,
    kind: 'diagnostic',
    phase: 'sample',
    requestedCodec: 'h265',
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
  assert.throws(() => parseStreamDiagnostic({ ...value, layers: Array(13).fill({}) }));
  assert.throws(() => parseStreamDiagnostic({ ...value, requestedCodec: 'private value' }));
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
