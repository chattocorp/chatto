<!--
@component

A small dialog that asks the user to confirm an action. Built on top of
`FormDialog` so it shares the same chrome (footer divider, button layout,
Enter-to-confirm) and tone-driven button color.

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
  import * as m from '$lib/i18n/messages';
  import FormDialog from './FormDialog.svelte';

  type Tone = 'danger' | 'warning' | 'info';

  let {
    children,
    visible = $bindable(true),
    title,
    tone = 'danger',
    actionLabel = m['common.confirm'](),
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

  const defaultIcons: Record<Tone, string> = {
    danger: 'iconify uil--exclamation-triangle',
    warning: 'iconify uil--exclamation-triangle',
    info: 'iconify uil--check'
  };

  const resolvedIcon = $derived(actionIcon ?? defaultIcons[tone]);
</script>

<FormDialog
  bind:visible
  {title}
  size="sm"
  submitLabel={actionLabel}
  submitTone={tone}
  submitIcon={resolvedIcon}
  submitLoadingText={m['ui.dialog.submit_loading']({ label: actionLabel })}
  {loading}
  onsubmit={() => onconfirm()}
  {onclose}
>
  <p class="text-muted">{@render children()}</p>
</FormDialog>
