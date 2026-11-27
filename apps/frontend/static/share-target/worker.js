// The browser starts this scoped worker even when no Chatto window is open.
// A redirect carries only a random ticket. Files stay here until the picker
// takes them, or until the one-minute cold-start window ends.
const pending = new Map();
const maxBytes = 64 * 1024 * 1024;
const pickerPath = '/share';

/** Bound the whole multipart body before the browser decodes text or files. */
async function readShare(request) {
  if (!request.body || Number(request.headers.get('content-length')) > maxBytes) return null;
  let bytes = 0;
  const bounded = request.body.pipeThrough(
    new TransformStream({
      transform(chunk, controller) {
        bytes += chunk.byteLength;
        if (bytes > maxBytes) throw new RangeError();
        controller.enqueue(chunk);
      }
    })
  );
  const form = await new Response(bounded, { headers: request.headers }).formData();
  const files = form.getAll('images');
  if (
    files.length > 10 ||
    files.some((file) => !(file instanceof File) || !file.type.startsWith('image/'))
  )
    return null;

  const field = (name) => {
    const value = form.get(name);
    return typeof value === 'string' ? value.trim() : '';
  };
  const parts = new Set([field('title'), field('text')].filter(Boolean));
  const url = field('url');
  // Android senders often include the URL in the text field as well.
  if (url && ![...parts].some((part) => part.split(/\s+/).includes(url))) parts.add(url);
  const text = [...parts].join('\n');
  return text || files.length ? { text, files } : null;
}

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (
    request.method !== 'POST' ||
    request.mode !== 'navigate' ||
    request.url !== new URL('./receive', self.registration.scope).href
  )
    return;

  const picker = new URL(pickerPath, self.location.origin);
  if (pending.size >= 3) {
    event.respondWith(Response.redirect(picker, 303));
    return;
  }
  const id = crypto.randomUUID();
  // Reserve capacity before reading. Hold the worker through both parsing and
  // the new page's startup, including when the browser starts a fresh process.
  let finish;
  event.waitUntil(
    new Promise((resolve) => {
      finish = resolve;
    })
  );
  const timer = setTimeout(release, 60_000);
  function release() {
    clearTimeout(timer);
    pending.delete(id);
    finish();
  }
  pending.set(id, null);
  event.respondWith(
    (async () => {
      const share = await readShare(request).catch(() => null);
      if (share && pending.has(id)) {
        pending.set(id, { share, release });
        picker.hash = id;
      } else {
        release();
      }
      return Response.redirect(picker, 303);
    })()
  );
});

self.addEventListener('message', (event) => {
  const port = event.ports[0];
  if (!port || !event.source?.url || new URL(event.source.url).pathname !== pickerPath) return;
  const entry = pending.get(event.data);
  port.postMessage(entry?.share ?? null);
  entry?.release();
});
