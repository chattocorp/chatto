<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { Button } from '$lib/ui/form';
  import { Dialog, Hint } from '$lib/ui';
  import { toast } from '$lib/ui/toast';

  let {
    visible = $bindable(false),
    value = $bindable(''),
    title,
    warning,
    copiedMessage,
    onclose
  }: {
    visible?: boolean;
    value?: string;
    title: string;
    warning: string;
    copiedMessage: string;
    onclose?: () => void;
  } = $props();

  async function copyCredential() {
    await navigator.clipboard.writeText(value);
    toast.success(copiedMessage);
  }

  function close() {
    visible = false;
    value = '';
    onclose?.();
  }
</script>

<!-- @component Shows one newly issued credential and clears it when the dialog closes. -->
<Dialog bind:visible {title} size="lg" onclose={close}>
  <div class="flex flex-col gap-4">
    <Hint tone="warning">{warning}</Hint>
    <div class="flex items-center gap-3 surface-box p-3">
      <code class="min-w-0 flex-1 overflow-x-auto text-sm whitespace-nowrap select-all" dir="ltr"
        >{value}</code
      >
      <Button size="sm" variant="secondary" onclick={copyCredential}>
        <span class="iconify icon-[uil--copy]" aria-hidden="true"></span>
        {m('common.copy_to_clipboard')}
      </Button>
    </div>
    <div class="flex justify-end">
      <Button defaultAction onclick={close}>{m('common.got_it')}</Button>
    </div>
  </div>
</Dialog>
