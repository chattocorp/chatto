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

  const toneButtonClasses: Record<Tone, string> = {
    danger: 'bg-danger text-white hover:bg-danger/90',
    warning: 'bg-warning text-white hover:bg-warning/90',
    info: 'bg-primary text-white hover:bg-primary-hover'
  };

  const defaultIcons: Record<Tone, string> = {
    danger: 'iconify uil--exclamation-triangle',
    warning: 'iconify uil--exclamation-triangle',
    info: 'iconify uil--check'
  };

  const resolvedIcon = $derived(actionIcon ?? defaultIcons[tone]);
</script>

<Dialog {visible} {title} size="sm" {onclose}>
  <p class="mb-4 px-3.5 text-muted">
    {@render children()}
  </p>
  <div class="flex justify-end gap-3">
    <button
      type="button"
      class="flex cursor-pointer items-center gap-2 rounded-lg bg-surface-200 px-4 py-2 text-sm font-medium text-text hover:bg-surface-300"
      onclick={onclose}
      disabled={loading}
    >
      <span class="iconify uil--times"></span>
      Cancel
    </button>
    <button
      type="button"
      class={[
        'flex cursor-pointer items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium disabled:opacity-50',
        toneButtonClasses[tone]
      ]}
      onclick={onconfirm}
      disabled={loading}
    >
      <span class={resolvedIcon}></span>
      {loading ? `${actionLabel}...` : actionLabel}
    </button>
  </div>
</Dialog>
