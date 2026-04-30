<!--
@component

A dialog wrapping a `<form>`. Owns the form element, the submit handler,
and a standard footer with cancel + submit buttons. Use this whenever a
modal dialog is collecting input — the submit button gets Enter-to-submit
for free and the boilerplate stays out of the calling component.

```svelte
<FormDialog
  bind:visible
  title="Create Room"
  submitLabel="Create"
  loading={isLoading}
  disabled={!name.trim()}
  onsubmit={handleSubmit}
  onclose={() => (visible = false)}
>
  <TextInput id="name" label="Name" bind:value={name} />
  <TextArea id="desc" label="Description" bind:value={description} />
</FormDialog>
```

The submit button's color follows `submitTone` (`primary` by default; use
`danger` for destructive forms like "Delete account, type to confirm").
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import Dialog from './Dialog.svelte';
  import { Button } from './form';

  type SubmitTone = 'primary' | 'danger';

  let {
    children,
    description,
    visible = $bindable(false),
    title,
    size = 'md',
    submitLabel = 'Save',
    submitTone = 'primary',
    submitLoadingText,
    cancelLabel = 'Cancel',
    loading = false,
    disabled = false,
    onsubmit,
    onclose
  }: {
    children: Snippet;
    /** Optional copy rendered above the form fields. */
    description?: Snippet;
    visible?: boolean;
    title: string;
    size?: 'sm' | 'md' | 'lg';
    submitLabel?: string;
    /** Visual weight of the submit button. */
    submitTone?: SubmitTone;
    /** Optional override for the submit button label while `loading`. */
    submitLoadingText?: string;
    cancelLabel?: string;
    loading?: boolean;
    /** Disables the submit button (e.g., when validation fails). */
    disabled?: boolean;
    onsubmit: (e: SubmitEvent) => void;
    onclose: () => void;
  } = $props();

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (loading || disabled) return;
    onsubmit(e);
  }

  const submitVariant = $derived<'primary' | 'danger'>(submitTone);
</script>

<Dialog bind:visible {title} {size} {onclose}>
  <form onsubmit={handleSubmit} class="flex flex-col gap-5">
    {#if description}
      <div class="text-muted">
        {@render description()}
      </div>
    {/if}

    {@render children()}

    <div class="-mx-3 h-px bg-text/10" aria-hidden="true"></div>

    <footer class="flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={onclose} disabled={loading}>
        {cancelLabel}
      </Button>
      <Button
        type="submit"
        variant={submitVariant}
        loading={loading}
        loadingText={submitLoadingText}
        disabled={disabled}
      >
        {submitLabel}
      </Button>
    </footer>
  </form>
</Dialog>
