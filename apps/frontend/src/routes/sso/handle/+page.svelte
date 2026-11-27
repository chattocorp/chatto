<!--
@component
Asks for the AT Protocol handle when a provider sign-in starts without one,
then continues the same sign-in (FDR-058). It runs in the sign-in window.
-->
<script lang="ts">
  import { page } from '$app/state';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { getAuthServerInfo } from '$lib/components/authServerInfo';
  import { m } from '$lib/i18n/messages';
  import { Hint, PageTitle } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import { atprotoHandleContinuationUrl } from '$lib/atproto/handleContinuation';
  import {
    forgetAtprotoHandle,
    rememberedAtprotoHandle,
    setPendingAtprotoHandle
  } from '$lib/atproto/rememberedHandle';

  import blueskyMark from '$lib/assets/atmosphere/bluesky.svg?url';
  import blackskyMark from '$lib/assets/atmosphere/blacksky.svg?url';
  import grazeMark from '$lib/assets/atmosphere/graze.svg?url';
  import leafletMark from '$lib/assets/atmosphere/leaflet.svg?url';
  import tangledMark from '$lib/assets/atmosphere/tangled.svg?url';
  import musocialMark from '$lib/assets/atmosphere/mu-social.svg?url';
  import surfMark from '$lib/assets/atmosphere/surf.svg?url';
  import pcktMark from '$lib/assets/atmosphere/pckt.svg?url';

  const services = [
    { name: 'Bluesky', mark: blueskyMark },
    { name: 'Blacksky', mark: blackskyMark },
    { name: 'Graze', mark: grazeMark },
    { name: 'Leaflet', mark: leafletMark },
    { name: 'Tangled', mark: tangledMark },
    { name: 'mu.social', mark: musocialMark },
    { name: 'Surf', mark: surfMark },
    { name: 'Pckt', mark: pcktMark }
  ];

  const getServerInfo = getAuthServerInfo();
  const next = page.url.searchParams.get('next');
  const notFound = page.url.searchParams.get('error') === 'atproto_identity_not_found';
  const initialHandle = rememberedAtprotoHandle();
  let remembered = $state(initialHandle);
  let handle = $state(page.url.searchParams.get('handle') ?? initialHandle);
  let submitting = $state(false);

  function forgetHandle() {
    forgetAtprotoHandle();
    remembered = '';
    handle = '';
  }

  function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    const target = atprotoHandleContinuationUrl(next, handle, window.location.origin);
    if (!target || !handle.trim()) return;
    submitting = true;
    setPendingAtprotoHandle(handle);
    window.location.href = target;
  }
</script>

<PageTitle title={m('auth.login.atproto_handle_title')} />

<AuthLayout compact title={m('auth.login.atproto_handle_title')}>
  {#if !atprotoHandleContinuationUrl(next, 'x', page.url.origin)}
    <Hint tone="danger">{m('auth.sso.invalid')}</Hint>
  {:else}
    <ul
      class="mx-auto mb-4 flex max-w-xs flex-wrap items-center justify-center gap-3 text-muted sm:gap-4"
    >
      {#each services as service (service.name)}
        <li title={service.name}>
          <span class="sr-only">{service.name}</span>
          <span
            aria-hidden="true"
            class="block size-5 bg-current mask-contain mask-center mask-no-repeat sm:size-6"
            style:mask-image={`url("${service.mark}")`}
          ></span>
        </li>
      {/each}
    </ul>
    <form class="flex flex-col gap-5" onsubmit={handleSubmit}>
      {#if notFound}
        <Hint tone="danger">{m('auth.login.error.atproto_identity_not_found')}</Hint>
      {/if}
      <div class="flex flex-col gap-2">
        <TextInput
          id="atproto-handle"
          label={m('auth.login.atproto_handle_label')}
          labelHidden
          placeholder={m('auth.login.atproto_handle_placeholder')}
          autocomplete="username"
          leadingIcon="icon-[mdi--at]"
          autofocus
          required
          bind:value={handle}
          disabled={submitting}
        />
        {#if remembered && handle === remembered}
          <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 px-1">
            <span class="text-sm text-muted">{m('auth.login.atproto_handle_saved')}</span>
            <button
              type="button"
              class="cursor-pointer text-sm link"
              disabled={submitting}
              onclick={forgetHandle}>{m('auth.remote.forget_atproto_handle')}</button
            >
          </div>
        {/if}
      </div>
      <Button type="submit" size="lg" fullWidth loading={submitting} disabled={!handle.trim()}>
        {m('common.continue')}
      </Button>
      <div class="mt-4">
        <Hint plain icon="icon-[uil--shield-check]">
          <p class="leading-relaxed text-pretty">
            {m('auth.login.atproto_handle_next', { server: getServerInfo()?.name || 'Chatto' })}
          </p>
        </Hint>
      </div>
    </form>
  {/if}
</AuthLayout>
