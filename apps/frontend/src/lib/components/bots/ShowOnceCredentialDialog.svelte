<script lang="ts">
  import { beforeNavigate } from '$app/navigation';
  import { m } from '$lib/i18n/messages';
  import { Button } from '$lib/ui/form';
  import { Dialog, Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import { errorMessage } from '$lib/utils/errorMessage';

  let {
    visible = $bindable(false),
    value = $bindable(''),
    title,
    warning,
    copiedMessage,
    pending = false,
    onclose
  }: {
    visible?: boolean;
    value?: string;
    title: string;
    warning: string;
    copiedMessage: string;
    pending?: boolean;
    onclose?: () => void;
  } = $props();

  let copyError = $state('');
  let copyAttempt = 0;

  beforeNavigate(({ cancel }) => {
    if (pending || (visible && !!value)) cancel();
  });

  async function copyCredential() {
    const attempt = ++copyAttempt;
    copyError = '';
    try {
      await navigator.clipboard.writeText(value);
      if (attempt === copyAttempt) toast.success(copiedMessage);
    } catch {
      if (attempt === copyAttempt) {
        copyError = errorMessage(undefined, m('common.copy_failed'));
      }
    }
  }

  function close() {
    // A clipboard result must not restore feedback after the credential is cleared.
    copyAttempt++;
    copyError = '';
    visible = false;
    value = '';
    onclose?.();
  }
</script>

<!-- @component Shows one newly issued credential, protects it from navigation, and clears it when the dialog closes. -->
<Dialog bind:visible {title} size="lg" onclose={close}>
  <div class="flex flex-col gap-4">
    <Hint tone="warning">{warning}</Hint>
    {#if copyError}
      <div role="alert"><Hint tone="danger">{copyError}</Hint></div>
    {/if}
    <div class="flex items-center gap-3 surface-box p-3">
      <code class="min-w-0 flex-1 overflow-x-auto text-sm whitespace-nowrap select-all" dir="ltr"
        >{value}</code
      >
      <Button size="sm" variant="secondary" onclick={copyCredential}>
        <span class="iconify icon-[uil--copy]" aria-hidden="true"></span>
        {m('common.copy_to_clipboard')}
      </Button>
    </div>
  </div>

  {#snippet primaryAction()}
    <Button defaultAction onclick={close}>{m('common.got_it')}</Button>
  {/snippet}
</Dialog>
