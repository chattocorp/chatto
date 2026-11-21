<!--
@component
Provider sign-in choices for the origin login and registration pages. A choice
opens the provider sign-in in the current window. The server returns to the
saved return path, or to the login page with an error.
-->
<script lang="ts">
  import type { PublicAuthProvider } from '@chatto/client/api/server';
  import AuthProviderButtons from './AuthProviderButtons.svelte';

  let {
    providers,
    href,
    compact = false,
    selectedProviderId = $bindable(null)
  }: {
    providers: readonly PublicAuthProvider[];
    href: (provider: PublicAuthProvider) => string;
    compact?: boolean;
    /** The provider whose sign-in is starting, or `null`. */
    selectedProviderId?: string | null;
  } = $props();
</script>

<!-- Back navigation can restore this page from the back/forward cache. -->
<svelte:window
  onpageshow={(event) => {
    if (event.persisted) selectedProviderId = null;
  }}
/>

<AuthProviderButtons
  {providers}
  {href}
  {compact}
  {selectedProviderId}
  onclick={(_, provider) => {
    selectedProviderId = provider.id;
  }}
/>
