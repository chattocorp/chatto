<!--
@component

A small dialog that asks the user to confirm an action.

Use the `tone` prop to communicate the weight of the action:

- `danger` (default) — destructive or irreversible (delete, leave, ban).
- `warning` — significant but reversible (kick from a call, force refresh).
- `info` — non-destructive confirmation (sign out, apply changes).

```svelte
<ConfirmDialog
  title="Sign Out"
  tone="info"
  actionLabel="Sign Out"
  actionIcon="iconify uil--signout"
  onconfirm={signOut}
  onclose={close}
>
  This will disconnect all instances and sign you out.
</ConfirmDialog>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import Dialog from './Dialog.svelte';
  import { Button } from './form';

  type Tone = 'danger' | 'warning' | 'info';

  let {
    children,
    visible = $bindable(true),
    title,
    tone = 'danger',
    actionLabel = 'Confirm',
    actionIcon,
    loading = false,
    onconfirm,
    onclose
  }: {
    children: Snippet;
    visible?: boolean;
    title: string;
    /** Communicates the weight of the action. Drives the confirm button's color and default icon. */
    tone?: Tone;
    actionLabel?: string;
    /** Iconify class for the confirm button. Defaults to a sensible icon per tone. */
    actionIcon?: string;
    loading?: boolean;
    onconfirm: () => void;
    onclose: () => void;
  } = $props();

  const toneVariants = {
    danger: 'danger',
    warning: 'warning',
    info: 'accent'
  } as const;

  const defaultIcons: Record<Tone, string> = {
    danger: 'iconify uil--exclamation-triangle',
    warning: 'iconify uil--exclamation-triangle',
    info: 'iconify uil--check'
  };

  const resolvedIcon = $derived(actionIcon ?? defaultIcons[tone]);

  // Link the body copy to the dialog so screen readers announce it on open.
  const confirmDialogId = $props.id();
  const messageId = `${confirmDialogId}-message`;
</script>

<Dialog {visible} {title} size="sm" describedBy={messageId} {onclose}>
  <p id={messageId} class="mb-4 px-2 text-muted">
    {@render children()}
  </p>
  <div class="flex justify-end gap-2">
    <Button variant="ghost" onclick={onclose} disabled={loading}>Cancel</Button>
    <Button
      variant={toneVariants[tone]}
      onclick={onconfirm}
      {loading}
      loadingText={`${actionLabel}...`}
    >
      <span class={resolvedIcon}></span>
      {actionLabel}
    </Button>
  </div>
</Dialog>
