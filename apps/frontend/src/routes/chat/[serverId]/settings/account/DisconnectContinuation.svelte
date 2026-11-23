<!--
@component
Consumes a disconnect continuation once, after account data is ready. Account
settings return here after a fresh-auth re-verification for a disconnect.
The page shows the disconnect confirmation again; the URL never disconnects
an identity without a click.
-->
<script lang="ts" module>
  const IDENTITY_PARAM = 'disconnect_identity';
  const USER_PARAM = 'disconnect_user';

  /** Settings path that resumes a disconnect after the provider callback. */
  export function disconnectContinuationPath(
    settingsPath: string,
    subjectHash: string,
    userId: string
  ): string {
    const url = new URL(settingsPath, 'http://localhost');
    url.searchParams.set(IDENTITY_PARAM, subjectHash);
    url.searchParams.set(USER_PARAM, userId);
    return url.pathname + url.search;
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte';
  import { base, resolve } from '$app/paths';
  import { page } from '$app/state';
  import { replaceState } from '$app/navigation';
  import { REVERIFIED_PARAM } from '$lib/auth/freshAuthReverification';

  let {
    ondisconnect
  }: {
    /** errorCode is the re-verification error, or empty after a success. */
    ondisconnect: (subjectHash: string, userId: string, errorCode: string) => void;
  } = $props();

  onMount(() => {
    const url = new URL(page.url);
    const subjectHash = url.searchParams.get(IDENTITY_PARAM) ?? '';
    if (!subjectHash) return;
    const userId = url.searchParams.get(USER_PARAM) ?? '';
    const errorCode = url.searchParams.get('error') ?? '';
    for (const name of [IDENTITY_PARAM, USER_PARAM, REVERIFIED_PARAM, 'error'])
      url.searchParams.delete(name);
    replaceState(
      resolve((url.pathname.slice(base.length) + url.search + url.hash) as '/'),
      page.state
    );
    ondisconnect(subjectHash, userId, errorCode);
  });
</script>
