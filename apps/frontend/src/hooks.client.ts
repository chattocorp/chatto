import type { ClientInit } from '@sveltejs/kit';
import { startLoadingGradients } from '$lib/ui/loadingGradients';

/** Start the loading animation before the initial route resolves. */
export const init: ClientInit = () => {
  const shell = document.getElementById('app-loading');
  if (!shell) return;
  const stop = startLoadingGradients(shell);
  if (import.meta.hot) import.meta.hot.dispose(stop);
};
