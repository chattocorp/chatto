<script lang="ts">
  import type { Snippet } from 'svelte';
  import { m } from '$lib/i18n/messages';
  import FieldFootnote from './FieldFootnote.svelte';

  let {
    label,
    id,
    error,
    description,
    required = false,
    labelHidden = false,
    group = false,
    children
  }: {
    label: string;
    id?: string;
    error?: string;
    description?: string;
    required?: boolean;
    /** Keep the label available to assistive technology without displaying it. */
    labelHidden?: boolean;
    /**
     * Label several controls that form one value, such as a date and a time.
     * Renders a fieldset with a legend; give each control its own accessible name.
     */
    group?: boolean;
    children: Snippet;
  } = $props();

  const labelClass = $derived(labelHidden ? 'sr-only' : 'text-sm font-medium text-text');
</script>

{#snippet labelText()}
  {label}{#if required}<span
      class="iconify ms-1 icon-[uil--asterisk] align-middle text-[0.7em] text-action"
      aria-hidden="true"
      title={m('ui.form.required')}
    ></span>{/if}
{/snippet}

{#if group}
  <fieldset class="flex min-w-0 flex-col gap-1.5">
    <legend class={['mb-1.5', labelClass]}>{@render labelText()}</legend>
    {@render children()}
    <FieldFootnote {id} {error} {description} />
  </fieldset>
{:else}
  <div class="flex flex-col gap-1.5">
    <label for={id} class={labelClass}>{@render labelText()}</label>
    {@render children()}
    <FieldFootnote {id} {error} {description} />
  </div>
{/if}
