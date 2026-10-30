<script lang="ts">
  import { onDestroy } from 'svelte';
  import { createMemberInvitationAPI } from '@chatto/client/api/invitations';
  import { m } from '$lib/i18n/messages';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { CopyButton, Dialog, Hint } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';

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
<Dialog visible title={m('chat.invitation.title')} size="sm" oncloserequest={clear} {onclose}>
  <div class="space-y-4 p-5">
    <p class="text-sm text-muted">{m('chat.invitation.description')}</p>
    {#if code}
      <div class="flex items-center justify-between gap-2">
        <code dir="ltr" class="font-mono text-base tracking-wide break-all">{code}</code>
        <CopyButton value={code} />
      </div>
      <Hint>{m('chat.invitation.once')}</Hint>
    {/if}
    {#if error}<Hint tone="danger">{error}</Hint>{/if}
  </div>
  {#snippet primaryAction()}
    {#if !code}
      <Button variant="action" loading={pending} disabled={pending} onclick={create}>
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
