import { afterEach, expect, it } from 'vitest';

/** Real same-origin realms exercise native Web Locks and shared storage. */
const frames: HTMLIFrameElement[] = [];
const cleanups: (() => void)[] = [];
const attemptKey = 'chatto.push-registration.browser-attempt.v1';

async function openGateFrame() {
  const frame = document.createElement('iframe');
  frames.push(frame);
  const id = crypto.randomUUID();
  const messages: string[] = [];
  const listener = (event: MessageEvent<{ id: string; result: string }>) => {
    if (event.source === frame.contentWindow && event.data?.id === id)
      messages.push(event.data.result);
  };
  window.addEventListener('message', listener);
  cleanups.push(() => window.removeEventListener('message', listener));
  // Only test-owned code and constants enter the isolated realm. The callback
  // emulates PushManager without contacting an external push service.
  const moduleUrl = new URL('./browserPushRegistration.ts', import.meta.url).href;
  frame.srcdoc = `<script type="module">
    import { createBrowserPushSubscription } from ${JSON.stringify(moduleUrl)};
    const send = result => parent.postMessage({id:${JSON.stringify(id)},result}, ${JSON.stringify(location.origin)});
    let release;
    window.addEventListener('message', async event => {
      if (event.source !== parent) return;
      if (event.data === 'fail') { release?.(); return; }
      if (event.data !== 'start') return;
      try {
        await createBrowserPushSubscription({
          cancelled: () => false,
          existing: async () => null,
          subscribe: async () => {
            send('creating');
            await new Promise(resolve => { release = resolve; });
            throw new Error('test browser failure');
          }
        });
      } catch (error) { send(error.name); }
    });
    send('ready');
  </script>`;
  document.body.append(frame);
  await expect.poll(() => messages).toContain('ready');
  return {
    frame,
    messages,
    send: (command: string) => frame.contentWindow!.postMessage(command, location.origin)
  };
}

afterEach(() => {
  for (const frame of frames.splice(0)) frame.remove();
  for (const cleanup of cleanups.splice(0)) cleanup();
  localStorage.removeItem(attemptKey);
});

it('stops another realm and a newly loaded realm after browser creation fails', async () => {
  localStorage.removeItem(attemptKey);
  const first = await openGateFrame();
  const second = await openGateFrame();
  first.send('start');
  await expect.poll(() => first.messages).toContain('creating');
  second.send('start');
  first.send('fail');
  await expect.poll(() => first.messages).toContain('Error');
  await expect.poll(() => second.messages).toContain('BrowserPushRegistrationPaused');
  expect(second.messages).not.toContain('creating');

  first.frame.remove();
  second.frame.remove();
  const reloaded = await openGateFrame();
  reloaded.send('start');
  await expect.poll(() => reloaded.messages).toContain('BrowserPushRegistrationPaused');
  expect(reloaded.messages).not.toContain('creating');
});

it('retains the stop when a realm exits before its browser call settles', async () => {
  localStorage.removeItem(attemptKey);
  const first = await openGateFrame();
  first.send('start');
  await expect.poll(() => first.messages).toContain('creating');
  first.frame.remove();
  const reloaded = await openGateFrame();
  reloaded.send('start');
  await expect.poll(() => reloaded.messages).toContain('BrowserPushRegistrationPaused');
  expect(reloaded.messages).not.toContain('creating');
});
