<!--
@component
Window bar of a standalone sign-in or authorization window. It replaces the
app header and shows the server's icon and name beside the server's address,
so that a user can confirm where they sign in. The page content inside the
frame shows the banner, the description, and the task.
-->
<script lang="ts">
  import { page } from '$app/state';
  import { getAuthServerInfo } from '$lib/components/authServerInfo';
  import { m } from '$lib/i18n/messages';

  const getServerInfo = getAuthServerInfo();
  const serverInfo = $derived(getServerInfo());
</script>

<header data-testid="sign-in-window-header" class="@container shrink-0 bg-surface text-muted">
  <div class="flex h-[var(--app-header-height)] items-center justify-between gap-3 px-3">
    <span class="flex min-w-0 items-center gap-2 font-semibold text-text">
      {#if serverInfo?.iconUrl}
        <img src={serverInfo.iconUrl} alt="" class="size-6 shrink-0 rounded-md" />
      {/if}
      <span class="truncate"><bdi>{serverInfo?.name || 'Chatto'}</bdi></span>
    </span>
    <!-- Narrow windows show only the name; the browser shows the address there.
       A long host loses its start, so the registrable domain at its end stays visible. -->
    <span class="flex max-w-[60%] min-w-0 shrink-0 items-center gap-1.5 @max-[26rem]:hidden">
      <span
        aria-hidden="true"
        class={[
          'iconify shrink-0',
          page.url.protocol === 'https:' ? 'icon-[uil--lock]' : 'icon-[uil--globe]'
        ]}
      ></span>
      <span class="sr-only">{m('auth.window.address_label')}</span>
      <span class="truncate [direction:rtl]"><bdi dir="ltr">{page.url.host}</bdi></span>
    </span>
  </div>
</header>
