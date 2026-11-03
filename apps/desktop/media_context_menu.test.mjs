import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { test } from 'node:test';
import { configureMediaContextMenu } from './media_context_menu.mjs';

function setup(origin = 'chatto://desktop/chat/server/room') {
  const contents = new EventEmitter();
  const copied = [];
  const downloads = [];
  const menus = [];
  contents.getURL = () => origin;
  contents.copyImageAt = (x, y) => copied.push([x, y]);
  contents.downloadURL = (url) => downloads.push(url);
  const window = { webContents: contents };
  configureMediaContextMenu(window, {
    buildFromTemplate(items) {
      return { popup: (options) => menus.push({ items, options }) };
    }
  });
  return { contents, window, copied, downloads, menus };
}

test('image menu copies the hit image and saves its source only on demand', () => {
  const app = setup();
  const frame = {};
  const srcURL = 'https://assets.example/image?access=ticket';
  app.contents.emit(
    'context-menu',
    {},
    {
      x: 30,
      y: 40,
      frame,
      mediaType: 'image',
      hasImageContents: true,
      srcURL
    }
  );
  const { items, options } = app.menus[0];
  assert.deepEqual(
    items.map((item) => item.label),
    ['Copy image', 'Save image as…']
  );
  assert.equal(items[0].enabled, true);
  assert.deepEqual(options, { window: app.window, frame });
  assert.deepEqual(app.copied, []);
  assert.deepEqual(app.downloads, []);
  items[0].click();
  assert.deepEqual(app.copied, [[30, 40]]);
  assert.deepEqual(app.downloads, []);
  items[1].click();
  assert.deepEqual(app.downloads, [srcURL]);
});

test('file links use the native download path and broken images disable copy', () => {
  const app = setup();
  const linkURL = 'https://assets.example/report.pdf?access=ticket&download=1';
  app.contents.emit('context-menu', {}, { mediaType: 'none', linkURL });
  assert.deepEqual(
    app.menus[0].items.map((item) => item.label),
    ['Save link as…']
  );
  app.menus[0].items[0].click();
  assert.deepEqual(app.downloads, [linkURL]);
  app.contents.emit('context-menu', {}, { mediaType: 'image', hasImageContents: false });
  assert.equal(app.menus[1].items[0].enabled, false);
});

test('local diagram sources can be saved without another server request', () => {
  for (const srcURL of ['blob:chatto://desktop/snapshot', 'data:image/svg+xml,<svg/>']) {
    const app = setup();
    app.contents.emit('context-menu', {}, { mediaType: 'image', hasImageContents: true, srcURL });
    app.menus[0].items[1].click();
    assert.deepEqual(app.downloads, [srcURL]);
  }
});

test('unrelated targets and unsafe file-link schemes have no native media menu', () => {
  const app = setup();
  for (const linkURL of [
    undefined,
    '',
    'file:///private/secret',
    'javascript:alert(1)',
    'chatto://desktop/chat',
    'invalid'
  ]) {
    app.contents.emit('context-menu', {}, { mediaType: 'none', linkURL });
  }
  assert.deepEqual(app.menus, []);
});

test('external pages cannot invoke native media actions', () => {
  const app = setup('https://example.com');
  app.contents.emit(
    'context-menu',
    {},
    {
      mediaType: 'image',
      hasImageContents: true,
      srcURL: 'https://example.com/image.png'
    }
  );
  assert.deepEqual(app.menus, []);
});
