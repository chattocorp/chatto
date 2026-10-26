import assert from 'node:assert/strict';
import test from 'node:test';
import { configureYouTubeEmbeds } from './youtube.mjs';

function hook() {
  let listener;
  configureYouTubeEmbeds({
    webRequest: {
      onBeforeSendHeaders(filter, callback) {
        assert.deepEqual(filter.urls, ['https://www.youtube-nocookie.com/embed/*']);
        listener = callback;
      }
    }
  });
  return (details) => {
    let result;
    listener(details, (response) => {
      result = response;
    });
    return result;
  };
}

test('identifies local YouTube frames without exposing a room or server', () => {
  const headers = { Accept: 'text/html', referer: 'chatto://desktop/private-room' };
  const result = hook()({
    resourceType: 'subFrame',
    frame: { parent: { url: 'chatto://desktop/private-room' } },
    requestHeaders: headers
  });
  assert.deepEqual(result, {
    requestHeaders: { Accept: 'text/html', Referer: 'https://run.chatto.desktop/' }
  });
  assert.equal(headers.referer, 'chatto://desktop/private-room');
});

test('supplies the identity when Chromium omits the referrer', () => {
  assert.deepEqual(
    hook()({
      resourceType: 'subFrame',
      frame: { parent: { url: 'chatto://desktop/' } },
      requestHeaders: { Accept: 'text/html' }
    }),
    { requestHeaders: { Accept: 'text/html', Referer: 'https://run.chatto.desktop/' } }
  );
});

test('does not change remote frames, top-level navigation, or player subresources', () => {
  const handle = hook();
  for (const details of [
    { resourceType: 'mainFrame', frame: { parent: null } },
    { resourceType: 'xhr', frame: { parent: { url: 'chatto://desktop' } } },
    { resourceType: 'subFrame', frame: { parent: { url: 'https://server.example' } } },
    { resourceType: 'subFrame', frame: { parent: { url: 'https://www.youtube-nocookie.com' } } },
    { resourceType: 'subFrame', frame: { parent: { url: 'chatto://desktop.evil' } } },
    { resourceType: 'subFrame', frame: null },
    { resourceType: 'subFrame' }
  ]) {
    assert.deepEqual(handle(details), {});
  }
});
