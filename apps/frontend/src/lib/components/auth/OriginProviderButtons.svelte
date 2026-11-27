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
    showRestrictions = false,
    selectedProviderId = $bindable(null)
  }: {
    providers: readonly PublicAuthProvider[];
    href: (provider: PublicAuthProvider) => string;
    compact?: boolean;
    /** Explain sign-in-only methods when account creation is available. */
    showRestrictions?: boolean;
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
  {showRestrictions}
  {selectedProviderId}
  onclick={(_, provider) => {
    selectedProviderId = provider.id;
  }}
/>
