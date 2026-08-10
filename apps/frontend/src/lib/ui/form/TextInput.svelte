<script lang="ts">
  import FormField from './FormField.svelte';
  import type { HTMLInputAttributes } from 'svelte/elements';

  let {
    label,
    id,
    testid,
    type = 'text',
    value = $bindable(''),
    placeholder,
    error,
    description,
    required = false,
    labelHidden = false,
    disabled = false,
    autocomplete,
    minlength,
    maxlength,
    autofocus = false,
    leadingIcon,
    trailingText,
    onkeydown,
    oninput
  }: {
    label: string;
    id?: string;
    testid?: string;
    type?: 'text' | 'email' | 'password' | 'url' | 'tel';
    value?: string;
    placeholder?: string;
    error?: string;
    description?: string;
    required?: boolean;
    /** Keep the label available to assistive technology without displaying it. */
    labelHidden?: boolean;
    disabled?: boolean;
    autocomplete?: HTMLInputAttributes['autocomplete'];
    minlength?: number;
    maxlength?: number;
    autofocus?: boolean;
    /** Iconify class name (e.g. `'icon-[uil--search]'`). Renders a leading icon inside the input. */
    leadingIcon?: string;
    /** Short trailing label rendered inside the input (e.g. a unit like `"px"`). */
    trailingText?: string;
    onkeydown?: (e: KeyboardEvent) => void;
    oninput?: (e: Event) => void;
  } = $props();
</script>

<FormField {label} {id} {error} {description} {required} {labelHidden}>
  <div class="relative">
    {#if leadingIcon}
      <span
        class={[
          'pointer-events-none absolute start-2 top-1/2 iconify -translate-y-1/2 text-base text-muted',
          leadingIcon
        ]}
        aria-hidden="true"
      ></span>
    {/if}
    <!-- svelte-ignore a11y_autofocus -->
    <input
      {id}
      data-testid={testid}
      {type}
      bind:value
      {placeholder}
      {required}
      {disabled}
      {autocomplete}
      {minlength}
      {maxlength}
      {autofocus}
      {onkeydown}
      {oninput}
      class={['input', leadingIcon && 'ps-8', trailingText && 'pe-10']}
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
    />
    {#if trailingText}
      <span
        class="pointer-events-none absolute end-2 top-1/2 -translate-y-1/2 text-sm text-muted"
        aria-hidden="true"
      >
        {trailingText}
      </span>
    {/if}
  </div>
</FormField>
