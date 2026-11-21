<!-- @component Consumes an identity-link or disconnect continuation once, after account data is ready. -->
<script lang="ts" module>
  /** Result of the provider round trip that returned to this page. */
  export type ContinuationState = {
    /** A fresh-auth re-verification succeeded just before this return. */
    reverified: boolean;
    /** Error code from the provider callback, or empty. */
    errorCode: string;
  };
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import { base, resolve } from '$app/paths';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';

  let {
    oncontinue,
    oncomplete,
    ondisconnect
  }: {
    oncontinue: (providerId: string, userId: string, state: ContinuationState) => void;
    /** errorCode is the server's link error, or empty after a successful link. */
    oncomplete: (providerId: string, userId: string, errorCode: string) => void;
    /** Resumes a disconnect that needed a fresh sign-in. */
    ondisconnect: (subjectHash: string, userId: string, state: ContinuationState) => void;
  } = $props();

  const continuationParams = [
    'link_provider',
    'link_user',
    'link_complete',
    'disconnect_identity',
    'reverified',
    'error'
  ];

  onMount(() => {
    const url = new URL(page.url);
    const disconnectSubjectHash = url.searchParams.get('disconnect_identity') ?? '';
    if (
      !url.searchParams.has('link_provider') &&
      !url.searchParams.has('link_user') &&
      !disconnectSubjectHash
    )
      return;
    const providerId = url.searchParams.get('link_provider') ?? '';
    const userId = url.searchParams.get('link_user') ?? '';
    const completed = url.searchParams.get('link_complete') === '1';
    const state: ContinuationState = {
      reverified: url.searchParams.get('reverified') === '1',
      errorCode: url.searchParams.get('error') ?? ''
    };
    for (const name of continuationParams) url.searchParams.delete(name);
    replaceState(
      resolve((url.pathname.slice(base.length) + url.search + url.hash) as '/'),
      page.state
    );
    if (disconnectSubjectHash) ondisconnect(disconnectSubjectHash, userId, state);
    else if (completed) oncomplete(providerId, userId, state.errorCode);
    else oncontinue(providerId, userId, state);
  });
</script>
