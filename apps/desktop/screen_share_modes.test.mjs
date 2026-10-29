import assert from 'node:assert/strict';
import { test } from 'node:test';
import { defaultScreenShareMode, screenShareModes } from './screen_share_modes.mjs';

test('the default requests tuned single-layer H.264 with automatic VBR fallback', () => {
  assert.equal(defaultScreenShareMode.codec, 'h264');
  assert.equal(defaultScreenShareMode.simulcast, false);
  assert.equal(defaultScreenShareMode.tunedH264, true);
  assert.equal(defaultScreenShareMode.rateControl, 'auto');
});

test('diagnostic selections retain all controls and cannot mutate an active share', () => {
  assert.deepEqual(
    screenShareModes.map(({ id }) => id),
    ['default', 'hevc', 'h264', 'single', 'average', 'vbr']
  );
  let selected = defaultScreenShareMode;
  const activeShare = selected;
  selected = screenShareModes.find(({ id }) => id === 'hevc');
  assert.equal(selected.codec, 'h265');
  assert.equal(activeShare.rateControl, 'auto');
  assert.throws(() => {
    activeShare.codec = 'h265';
  }, TypeError);
});
