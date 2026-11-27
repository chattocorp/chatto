<!-- @component Opens account creation when the server offers email registration. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import type { PublicServerInfo } from '@chatto/client/api/server';
  import { m } from '$lib/i18n/messages';

  let { serverInfo, redirectUrl }: { serverInfo: PublicServerInfo | null; redirectUrl: string } =
    $props();

  const returnQuery = $derived(
    redirectUrl === '/' ? '' : `&${new URLSearchParams({ redirect: redirectUrl })}`
  );
</script>

{#if serverInfo?.directRegistrationEnabled ?? true}
  <p class="mt-6 text-center">
    <a class="link" href={resolve(`/register?method=email${returnQuery}`)}
      >{m('auth.login.registration_link')}</a
    >
  </p>
{/if}
