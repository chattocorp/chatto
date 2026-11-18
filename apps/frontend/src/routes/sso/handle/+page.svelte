<!--
@component
Asks for the AT Protocol handle when a provider sign-in starts without one,
then continues the same sign-in (FDR-058). It runs in the sign-in window.
-->
<script lang="ts">
  import { page } from '$app/state';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import { Hint, PageTitle } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import { atprotoHandleContinuationUrl } from '$lib/atproto/handleContinuation';
  import {
    forgetAtprotoHandle,
    rememberedAtprotoHandle,
    setPendingAtprotoHandle
  } from '$lib/atproto/rememberedHandle';

  const next = page.url.searchParams.get('next');
  const notFound = page.url.searchParams.get('error') === 'atproto_identity_not_found';
  const remembered = rememberedAtprotoHandle();
  let handle = $state(page.url.searchParams.get('handle') ?? remembered);
  let submitting = $state(false);

  function forgetHandle() {
    forgetAtprotoHandle();
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

<AuthLayout title={m('auth.login.atproto_handle_title')}>
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
        placeholder={m('auth.login.atproto_handle_placeholder')}
        autocomplete="username"
        required
        bind:value={handle}
        disabled={submitting}
      />
      {#if remembered && handle === remembered}
        <button
          type="button"
          class="cursor-pointer self-start link"
          disabled={submitting}
          onclick={forgetHandle}>{m('auth.remote.forget_atproto_handle')}</button
        >
      {/if}
      <Button type="submit" size="lg" fullWidth loading={submitting} disabled={!handle.trim()}>
        {m('common.continue')}
      </Button>
    </form>
  {/if}
</AuthLayout>
