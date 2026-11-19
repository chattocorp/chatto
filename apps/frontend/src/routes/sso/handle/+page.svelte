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
    <form class="flex flex-col gap-4" onsubmit={handleSubmit}>
      {#if notFound}
        <Hint tone="danger">{m('auth.login.error.atproto_identity_not_found')}</Hint>
      {/if}
      <TextInput
        id="atproto-handle"
        label={m('auth.login.atproto_handle_label')}
        description={m('auth.login.atproto_handle_help', {
          example: '@' + m('auth.login.atproto_handle_placeholder')
        })}
        placeholder={m('auth.login.atproto_handle_placeholder')}
        autocomplete="username"
        leadingIcon="icon-[mdi--at]"
        autofocus
        required
        bind:value={handle}
        disabled={submitting}
      />
      {#if remembered && handle === remembered}
        <div class="-mt-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
          <span class="text-sm text-muted">{m('auth.login.atproto_handle_saved')}</span>
          <button
            type="button"
            class="cursor-pointer text-sm link"
            disabled={submitting}
            onclick={forgetHandle}>{m('auth.remote.forget_atproto_handle')}</button
          >
        </div>
      {/if}
      <Button type="submit" size="lg" fullWidth loading={submitting} disabled={!handle.trim()}>
        {m('common.continue')}
      </Button>
      <Hint plain icon="icon-[uil--shield-check]">
        {m('auth.login.atproto_handle_next', { server: getServerInfo()?.name || 'Chatto' })}
      </Hint>
    </form>
  {/if}
</AuthLayout>
