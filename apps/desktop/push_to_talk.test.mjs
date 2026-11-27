import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { PassThrough } from 'node:stream';
import { setImmediate as tick } from 'node:timers/promises';
import test from 'node:test';
import { configurePushToTalk } from './push_to_talk.mjs';

test('only the current call receives hold edges and helper failure releases its microphone', async () => {
  const contents = new EventEmitter();
  contents.isDestroyed = () => false;
  const events = [];
  contents.send = (_channel, token, edge) => events.push([token, edge]);
  const helpers = [];
  const powerMonitor = new EventEmitter();
  let set;
  configurePushToTalk({
    ipcMain: {
      handle: (_channel, handle) => {
        set = handle;
      }
    },
    contents,
    powerMonitor,
    isTrusted: (e) => e === 'main',
    executable: 'helper',
    appId: 'run.chatto.desktop',
    launch: () => {
      const child = new EventEmitter();
      child.stdout = new PassThrough();
      child.kill = () => queueMicrotask(() => child.emit('close', 0));
      helpers.push(child);
      return child;
    }
  });
  assert.equal(set('other', 'Control+M', 'a'), false);
  assert.equal(set('main', 'Alt+M', 'a'), false);
  assert.equal(set('main', 'Control+M', 'a'), true);
  await tick();
  helpers[0].stdout.write('ready\ndown\ndown\nup\nup\ndown\n');
  powerMonitor.emit('suspend');
  helpers[0].stdout.write('down\n');
  await tick();
  powerMonitor.emit('resume');
  await tick();
  helpers[1].stdout.write('ready\ndown\n');
  set('main', 'Control+N', 'b');
  set('main', '', 'a');
  await tick();
  helpers[2].stdout.write('ready\ndown\n');
  helpers[2].emit('close', 1);
  assert.deepEqual(events, [
    ['a', 'down'],
    ['a', 'up'],
    ['a', 'down'],
    ['a', 'up'],
    ['a', 'down'],
    ['a', 'up'],
    ['b', 'down'],
    ['b', 'failed']
  ]);
});
