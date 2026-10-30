<script lang="ts">
  import { onDestroy } from 'svelte';
  import { createMemberInvitationAPI } from '@chatto/client/api/invitations';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Dialog, Hint } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { copyToClipboard } from '$lib/utils/copyToClipboard';

  let { onclose }: { onclose: () => void } = $props();
  const scope = useServerScope();
  // The request and secret belong to this opening only. Back, permission loss,
  // or scope disposal unmounts this component; Forward opens an empty dialog.
  const controller = new AbortController();
  let code = $state('');
  let pending = $state(false);
  let error = $state<string | null>(null);

  function clear() {
    controller.abort();
    code = '';
  }
  onDestroy(clear);

  async function create() {
    if (pending || code || controller.signal.aborted) return;
    pending = true;
    error = null;
    try {
      const issued = await scope.connection.getAPI(createMemberInvitationAPI).create({
        signal: controller.signal
      });
      if (!controller.signal.aborted && scope.isCurrent() && scope.store.permissions.canInviteUsers)
        code = issued.code;
    } catch (cause) {
      if (!controller.signal.aborted) error = errorMessage(cause);
    } finally {
      pending = false;
    }
  }
</script>

<!-- @component One opening owns one code. No mutation cache or browser storage retains it. -->
<Dialog visible title={m('chat.invitation.title')} size="md" oncloserequest={clear} {onclose}>
  <div class="flex flex-col items-center gap-5 px-2 py-4 text-center">
    <span aria-hidden="true" class="iconify icon-[uil--envelope-share] text-5xl text-action"></span>
    {#if code}
      <code
        dir="ltr"
        class="block w-full font-mono text-3xl leading-snug font-semibold tracking-wide text-balance wrap-anywhere text-text-top"
        >{code}</code
      >
    {/if}
    <p class="max-w-md text-lg leading-relaxed text-pretty">{m('chat.invitation.description')}</p>
    {#if code}
      <p class="max-w-md text-base leading-relaxed text-pretty">{m('chat.invitation.once')}</p>
    {/if}
    {#if error}<Hint tone="danger">{error}</Hint>{/if}
  </div>
  {#snippet primaryAction()}
    {#if code}
      <Button variant="action" onclick={() => void copyToClipboard(code)}>
        <span aria-hidden="true" class="iconify icon-[uil--copy]"></span>
        {m('common.copy_to_clipboard')}
      </Button>
    {:else}
      <Button variant="action" loading={pending} disabled={pending} onclick={create}>
        <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
        {m('chat.invitation.create')}
      </Button>
    {/if}
  {/snippet}
  {#snippet dismissAction()}
    <Button
      variant="secondary"
      onclick={() => {
        clear();
        onclose();
      }}>{m('ui.close')}</Button
    >
  {/snippet}
</Dialog>
