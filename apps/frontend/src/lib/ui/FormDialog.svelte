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
  error={submitError}
  onsubmit={handleSubmit}
  onclose={() => (visible = false)}
>
  <TextInput id="name" label="Name" bind:value={name} />
  <TextArea id="desc" label="Description" bind:value={description} />
</FormDialog>
```

The submit button's color follows `submitTone` (`action` by default; use
`danger` for destructive forms like "Delete account, type to confirm").
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import * as m from '$lib/i18n/messages';
  import Dialog from './Dialog.svelte';
  import { Button, FormError } from './form';

  type SubmitTone = 'action' | 'warning' | 'danger';

  let {
    children,
    description,
    visible = $bindable(false),
    title,
    size = 'md',
    submitLabel = m['common.save'](),
    submitTone = 'action',
    submitIcon = 'iconify uil--check',
    submitLoadingText,
    cancelLabel = m['common.cancel'](),
    cancelIcon = 'iconify uil--times',
    loading = false,
    disabled = false,
    error,
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
    /**
     * Iconify class for the submit button. Defaults to a checkmark; pass an
     * action-specific icon for "Create" / "Delete" / "Connect" etc., or an
     * empty string to suppress.
     */
    submitIcon?: string;
    /** Optional override for the submit button label while `loading`. */
    submitLoadingText?: string;
    cancelLabel?: string;
    /** Iconify class for the cancel button. Pass an empty string to suppress. */
    cancelIcon?: string;
    loading?: boolean;
    /** Disables the submit button (e.g., when validation fails). */
    disabled?: boolean;
    /** Submission error to render below the form fields. */
    error?: string | null;
    onsubmit: (e: SubmitEvent) => void;
    onclose: () => void;
  } = $props();

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    if (loading || disabled) return;
    onsubmit(e);
  }

  const submitVariant = $derived<'action' | 'warning' | 'danger'>(
    submitTone === 'danger' ? 'danger' : submitTone === 'warning' ? 'warning' : 'action'
  );

  // Link the description copy to the dialog (only when present) so screen
  // readers announce it on open.
  const formDialogId = $props.id();
  const descriptionId = `${formDialogId}-description`;
</script>

<Dialog bind:visible {title} {size} describedBy={description ? descriptionId : undefined} {onclose}>
  <form onsubmit={handleSubmit} class="flex flex-col gap-5">
    {#if description}
      <div id={descriptionId} class="text-muted">
        {@render description()}
      </div>
    {/if}

    {@render children()}

    {#if error}
      <FormError {error} />
    {/if}

    <!--
      Footer "section": divider hugs the buttons, with pt-3 above the buttons
      to mirror the well's pb-3 below. -mx-3 cancels the well's px-3 so the
      divider extends to the well edges.
    -->
    <div class="-mx-3">
      <div class="h-px bg-text/10" aria-hidden="true"></div>
      <footer class="flex justify-end gap-2 px-3 pt-3">
        <Button type="button" variant="secondary" onclick={onclose} disabled={loading}>
          {#if cancelIcon}<span class={cancelIcon}></span>{/if}
          {cancelLabel}
        </Button>
        <Button
          type="submit"
          variant={submitVariant}
          {loading}
          loadingText={submitLoadingText}
          {disabled}
        >
          {#if submitIcon}<span class={submitIcon}></span>{/if}
          {submitLabel}
        </Button>
      </footer>
    </div>
  </form>
</Dialog>
