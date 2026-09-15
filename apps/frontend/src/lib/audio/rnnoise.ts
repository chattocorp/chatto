import workletURL from '@sapphi-red/web-noise-suppressor/rnnoiseWorklet.js?url';
import wasmURL from '@sapphi-red/web-noise-suppressor/rnnoise.wasm?url';
import simdURL from '@sapphi-red/web-noise-suppressor/rnnoise_simd.wasm?url';

const modules = new WeakMap<AudioContext, Promise<void>>();

/** Reject a deployment asset prefix that would send microphone-feature requests off-origin. */
function localURL(value: string): string {
  const url = new URL(value, location.href);
  if (url.origin !== location.origin) throw new Error('Noise suppression assets must be local');
  return url.href;
}

/** Bound module loading too: addModule has no AbortSignal parameter. */
function abortable<T>(work: Promise<T>, signal: AbortSignal): Promise<T> {
  return new Promise((resolve, reject) => {
    const abort = () => reject(new Error('RNNoise initialization cancelled'));
    signal.addEventListener('abort', abort, { once: true });
    if (signal.aborted) abort();
    void work.then(resolve, reject).finally(() => signal.removeEventListener('abort', abort));
  });
}

/** The patched upstream worklet acknowledges successful WASM setup before routing audio. */
function ready(node: AudioWorkletNode, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const finish = (error?: Error) => {
      node.port.removeEventListener('message', onMessage);
      node.removeEventListener('processorerror', onError);
      signal.removeEventListener('abort', onAbort);
      if (error) reject(error);
      else resolve();
    };
    const onMessage = (event: MessageEvent) => {
      if (event.data === 'ready') finish();
      if (event.data === 'error') finish(new Error('RNNoise initialization failed'));
    };
    const onError = () => finish(new Error('RNNoise worklet failed'));
    const onAbort = () => finish(new Error('RNNoise initialization cancelled'));
    node.port.addEventListener('message', onMessage);
    node.port.start();
    node.addEventListener('processorerror', onError);
    signal.addEventListener('abort', onAbort, { once: true });
    if (signal.aborted) onAbort();
  });
}

/** Optional RNNoise stage. Owns its nodes, never the caller's context or capture track. */
export class RnnoiseStage {
  readonly input: GainNode;
  unavailable = false;
  #node?: import('@sapphi-red/web-noise-suppressor').RnnoiseWorkletNode;
  #generation = 0;
  #enabled = false;
  #disposed = false;
  #abort?: AbortController;

  constructor(
    private context: AudioContext,
    private output: AudioNode
  ) {
    this.input = context.createGain();
    this.input.connect(output);
  }

  /** Load on demand. A disabled or failed stage passes input through unchanged. */
  async setEnabled(enabled: boolean): Promise<void> {
    if (this.#disposed || enabled === this.#enabled) return;
    this.#enabled = enabled;
    const generation = ++this.#generation;
    this.#abort?.abort();
    this.disconnectNode();
    this.unavailable = false;
    if (!enabled) return;
    // The upstream worklet processes 480-sample frames at 48 kHz, without resampling.
    if (this.context.sampleRate !== 48000) {
      this.unavailable = true;
      return;
    }
    const abort = new AbortController();
    this.#abort = abort;
    const timeout = setTimeout(() => abort.abort(), 10000);
    try {
      const { loadRnnoise, RnnoiseWorkletNode } = await import('@sapphi-red/web-noise-suppressor');
      if (generation !== this.#generation) return;
      let loaded = modules.get(this.context);
      if (!loaded) {
        loaded = this.context.audioWorklet.addModule(localURL(workletURL));
        modules.set(this.context, loaded);
        void loaded.catch(() => modules.delete(this.context));
      }
      const [wasmBinary] = await abortable(
        Promise.all([
          loadRnnoise(
            { url: localURL(wasmURL), simdUrl: localURL(simdURL) },
            {
              signal: abort.signal,
              credentials: 'same-origin',
              redirect: 'error'
            }
          ),
          loaded
        ]),
        abort.signal
      );
      if (generation !== this.#generation || this.#disposed) return;
      const node = new RnnoiseWorkletNode(this.context, { maxChannels: 1, wasmBinary });
      // Fold stereo microphones down only on the enabled processing path.
      node.channelCount = 1;
      node.channelCountMode = 'explicit';
      this.#node = node;
      await ready(node, abort.signal);
      if (generation !== this.#generation || this.#disposed) return;
      node.addEventListener('processorerror', this.onError, { once: true });
      this.input.disconnect();
      this.input.connect(node).connect(this.output);
    } catch {
      if (generation !== this.#generation || this.#disposed) return;
      this.disconnectNode();
      this.unavailable = true;
    } finally {
      clearTimeout(timeout);
    }
  }

  private onError = (): void => {
    this.disconnectNode();
    this.unavailable = true;
  };

  private disconnectNode(): void {
    this.input.disconnect();
    if (this.#node) {
      this.#node.removeEventListener('processorerror', this.onError);
      this.#node.destroy();
      this.#node.disconnect();
      this.#node.port.close();
      this.#node = undefined;
    }
    this.input.connect(this.output);
  }

  /** Fence pending asset loads and release the WASM processor before dropping its port. */
  destroy(): void {
    this.#disposed = true;
    this.#generation++;
    this.#abort?.abort();
    this.disconnectNode();
    this.input.disconnect();
  }
}
