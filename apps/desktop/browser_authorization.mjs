// The OS can deliver any chatto: URL. Only a live sign-in can claim its callback.
const callback = 'chatto://desktop/servers/callback?mode=popup';

/** Own the pending browser requests for one main renderer. Never persist a callback. */
export function createBrowserAuthorization(openExternal) {
  const pending = new Map();

  function cancel(state) {
    pending.get(state)?.reject(new Error('Browser sign-in ended.'));
  }

  return {
    async start(serverOrigin, value) {
      const url = URL.parse(value);
      const params = url?.searchParams;
      const state = params?.get('state');
      const local =
        url &&
        (['127.0.0.1', '[::1]', 'localhost'].includes(url.hostname) ||
          url.hostname.endsWith('.localhost'));
      if (
        !url ||
        url.origin !== serverOrigin ||
        url.username ||
        url.password ||
        url.hash ||
        (url.protocol !== 'https:' && !(url.protocol === 'http:' && local)) ||
        params.get('client_id') !== 'chatto://desktop' ||
        params.get('redirect_uri') !== callback ||
        !state ||
        pending.has(state)
      )
        throw new Error('Invalid browser sign-in request.');

      const request = Promise.withResolvers();
      pending.set(state, request);
      const timeout = setTimeout(() => cancel(state), 5 * 60 * 1000);
      try {
        // Register the waiter before launching. A browser with a session can return immediately.
        void openExternal(url.href).catch(request.reject);
        return await request.promise;
      } catch {
        throw new Error('Browser sign-in failed or timed out.');
      } finally {
        clearTimeout(timeout);
        pending.delete(state);
      }
    },
    /** Return true only when the OS supplied a callback to a waiting renderer. */
    accept(value) {
      const url = URL.parse(value);
      if (
        !url ||
        url.protocol !== 'chatto:' ||
        url.host !== 'desktop' ||
        url.username ||
        url.password ||
        url.hash ||
        url.pathname !== '/servers/callback' ||
        url.searchParams.getAll('mode').length !== 1 ||
        url.searchParams.get('mode') !== 'popup' ||
        url.searchParams.getAll('state').length !== 1
      )
        return false;
      const state = url.searchParams.get('state');
      const request = pending.get(state);
      if (!request) return false;
      pending.delete(state);
      request.resolve(url.href);
      return true;
    },
    cancel,
    clear() {
      for (const state of pending.keys()) cancel(state);
    }
  };
}
