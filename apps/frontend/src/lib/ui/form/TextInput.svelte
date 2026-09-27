<script lang="ts">
  import FormField from './FormField.svelte';
  import type { Snippet } from 'svelte';
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
    leading,
    trailing,
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
    /**
     * Interactive content inside the field at the inline start, such as an
     * emoji picker trigger. Use one `field-action` button; the input reserves
     * its width.
     */
    leading?: Snippet;
    /**
     * Interactive content inside the field at the inline end, such as a clear
     * button. Use one `field-action` button; the input reserves its width.
     */
    trailing?: Snippet;
    onkeydown?: (e: KeyboardEvent) => void;
    oninput?: (e: Event) => void;
  } = $props();
</script>

<FormField {label} {id} {error} {description} {required} {labelHidden}>
  <div class="relative">
    {#if leadingIcon}
      <span
        class={[
          'iconify pointer-events-none absolute start-2 top-1/2 -translate-y-1/2 text-base text-muted',
          leadingIcon
        ]}
        aria-hidden="true"
      ></span>
    {/if}
    <!-- Autofocus is opt-in through the autofocus prop; it defaults to false. -->
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
      class={[
        'input',
        leadingIcon && 'ps-8',
        leading && 'ps-10',
        (trailingText || trailing) && 'pe-10'
      ]}
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={error ? `${id}-error` : description ? `${id}-description` : undefined}
    />
    {#if leading}
      <div class="absolute inset-y-0 start-1 flex items-center">{@render leading()}</div>
    {/if}
    {#if trailing}
      <div class="absolute inset-y-0 end-1 flex items-center">{@render trailing()}</div>
    {/if}
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
