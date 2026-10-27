/** Worker isolation lets the host terminate a parser or layout that takes too long. */
import { renderMermaid } from './render';

self.onmessage = (event: MessageEvent<string>) => {
  try {
    self.postMessage(renderMermaid(event.data));
  } catch {
    // Parser errors can contain private message text. Return only the failure state.
    self.postMessage(null);
  }
};

// Static imports have completed before this handshake. The host starts the
// per-diagram execution timer only after the renderer can accept work.
self.postMessage({ type: 'ready' });
