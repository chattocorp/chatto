<script lang="ts">
  import type { Snippet } from 'svelte';
  import * as m from '$lib/i18n/messages';
  import FieldFootnote from './FieldFootnote.svelte';

  let {
    label,
    id,
    error,
    description,
    required = false,
    labelHidden = false,
    children
  }: {
    label: string;
    id?: string;
    error?: string;
    description?: string;
    required?: boolean;
    /** Keep the label available to assistive technology without displaying it. */
    labelHidden?: boolean;
    children: Snippet;
  } = $props();
</script>

<div class="flex flex-col gap-1.5">
  <label for={id} class={labelHidden ? 'sr-only' : 'text-sm font-medium text-text'}>
    {label}{#if required}<span
        class="ml-1 iconify align-middle text-[0.7em] text-action uil--asterisk"
        aria-hidden="true"
        title={m['ui.form.required']()}
      ></span>{/if}
  </label>

  {@render children()}

  <FieldFootnote {id} {error} {description} />
</div>
