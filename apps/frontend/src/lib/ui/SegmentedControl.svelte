<!--
@component

A compact one-of-many mode switch. Use this for alternate views, filters, and
sort modes that belong together, such as “All / Unread” or
“Most relevant / Newest”. Native radio inputs provide keyboard navigation and
selection semantics; the surrounding pill presents the options as one control.

Use `ToggleChip` instead when choices can be toggled independently.
-->
<script lang="ts" generics="T extends string | number">
  import type { ClassValue } from 'svelte/elements';

  let {
    label,
    options,
    value,
    onchange,
    disabled = false,
    class: className
  }: {
    /** Accessible name for the group. */
    label: string;
    options: ReadonlyArray<{ value: T; label: string; disabled?: boolean }>;
    value: T;
    onchange: (value: T) => void;
    disabled?: boolean;
    /** Layout-only classes such as responsive visibility or width. */
    class?: ClassValue;
  } = $props();

  const controlId = $props.id();
  const groupName = `segmented-control-${controlId}`;
</script>

<fieldset
  class={[
    'control-frame inline-flex h-10 w-fit min-w-0 items-center gap-px bg-input p-px',
    className
  ]}
  {disabled}
>
  <legend class="sr-only">{label}</legend>

  {#each options as option, index (option.value)}
    <label class="relative flex min-w-0 cursor-pointer">
      <input
        class="peer absolute inset-0 z-10 m-0 h-full w-full cursor-pointer appearance-none rounded-full opacity-0 disabled:cursor-not-allowed"
        type="radio"
        name={groupName}
        value={String(option.value)}
        checked={value === option.value}
        disabled={disabled || option.disabled}
        onchange={() => onchange(option.value)}
      />
      <span
        class={[
          'inline-flex min-h-9 min-w-10 items-center justify-center px-3 text-sm font-medium text-muted transition-[background-color,color] duration-150 peer-checked:bg-surface-selected peer-checked:text-text-top peer-[:not(:checked):hover]:bg-surface-emphasized/50 peer-[:not(:checked):hover]:text-text peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-action peer-disabled:cursor-not-allowed peer-disabled:opacity-60',
          index === 0 ? 'rounded-l' : '',
          index === options.length - 1 ? 'rounded-r' : ''
        ]}
      >
        {option.label}
      </span>
    </label>
  {/each}
</fieldset>
