<!--
@component

Standalone form layout for pages, panels, and auth flows. Use this when a form
is not a modal: it owns the vertical rhythm, optional max-width, optional
section separator, form-level error placement, and an optional footer area.

```svelte
<Form onsubmit={handleSubmit} maxWidth="max-w-md" error={submitError}>
  <TextInput id="name" label="Name" bind:value={name} />

  {#snippet footer()}
    <Button type="submit" disabled={!isDirty}>Save</Button>
  {/snippet}
</Form>
```
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLFormAttributes } from 'svelte/elements';
  import FormError from './FormError.svelte';

  type Spacing = 'compact' | 'default' | 'spacious';
  type MaxWidth = 'none' | 'max-w-sm' | 'max-w-md' | 'max-w-lg' | 'max-w-xl' | 'max-w-2xl';

  let {
    children,
    footer,
    error,
    action,
    method = 'post',
    autocomplete,
    novalidate = false,
    spacing = 'default',
    maxWidth = 'none',
    bordered = false,
    onsubmit
  }: {
    children: Snippet;
    footer?: Snippet;
    /** Form-level error rendered after the fields and before the footer. */
    error?: string | null;
    action?: string;
    method?: HTMLFormAttributes['method'];
    autocomplete?: HTMLFormAttributes['autocomplete'];
    novalidate?: boolean;
    spacing?: Spacing;
    maxWidth?: MaxWidth;
    /** Adds the standard top separator used for settings sections. */
    bordered?: boolean;
    onsubmit?: (e: SubmitEvent) => void;
  } = $props();

  const spacingClass = $derived(
    spacing === 'compact' ? 'gap-3' : spacing === 'spacious' ? 'gap-6' : 'gap-4'
  );
  const maxWidthClass = $derived(maxWidth === 'none' ? undefined : maxWidth);

  function handleSubmit(e: SubmitEvent) {
    if (!onsubmit) return;
    e.preventDefault();
    onsubmit(e);
  }
</script>

<form
  {action}
  {method}
  {autocomplete}
  {novalidate}
  onsubmit={handleSubmit}
  class={['flex flex-col', spacingClass, maxWidthClass, bordered && 'border-t border-border pt-6']}
>
  {@render children()}

  {#if error}
    <FormError {error} />
  {/if}

  {#if footer}
    <footer class="flex flex-wrap items-center gap-2 pt-1">
      {@render footer()}
    </footer>
  {/if}
</form>
