import {
  getViewerStateViaConnect,
  type ViewerAPIConfig,
  type ViewerState
} from '$lib/api-client/viewer';

type ViewerStateLoader = (config: ViewerAPIConfig) => Promise<ViewerState>;

/**
 * Owns the complete viewer snapshot for one registered server.
 *
 * Components consume this store instead of issuing their own GetViewer calls.
 * Concurrent consumers share the same load, while refresh() is the explicit
 * path for state that must be re-read after a mutation or reconnect.
 */
export class ViewerStateStore {
  data = $state.raw<ViewerState | null>(null);
  loading = $state(true);
  error = $state<unknown>(null);

  #pending: Promise<ViewerState> | null = null;

  constructor(
    private readonly config: ViewerAPIConfig,
    private readonly loader: ViewerStateLoader = getViewerStateViaConnect,
    private readonly onUpdate?: (viewer: ViewerState) => void
  ) {}

  seed(viewer: ViewerState): void {
    this.data = viewer;
    this.loading = false;
    this.error = null;
    this.onUpdate?.(viewer);
  }

  load(): Promise<ViewerState> {
    if (this.data) return Promise.resolve(this.data);
    return this.#request();
  }

  refresh(): Promise<ViewerState> {
    return this.#request();
  }

  clear(): void {
    this.data = null;
    this.loading = false;
    this.error = null;
    this.#pending = null;
  }

  #request(): Promise<ViewerState> {
    if (this.#pending) return this.#pending;

    this.loading = true;
    this.error = null;
    const request = this.loader(this.config);
    this.#pending = request;
    request.then(
      (viewer) => {
        if (this.#pending !== request) return;
        this.seed(viewer);
        this.#pending = null;
      },
      (error) => {
        if (this.#pending !== request) return;
        this.error = error;
        this.loading = false;
        this.#pending = null;
      }
    );
    return request;
  }
}
