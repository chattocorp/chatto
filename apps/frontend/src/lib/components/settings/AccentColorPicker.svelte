<!--
@component
Curated accent choices with native radio keyboard behaviour and a visible check.
The owner applies and saves changes. Samples retain their own palette colours.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { accentColors, type AccentColor } from '$lib/state/userPreferences.svelte';

  let {
    value,
    onchange
  }: {
    value: AccentColor;
    onchange: (value: AccentColor) => void;
  } = $props();
  const groupName = $props.id();
</script>

<div class="@container">
  <fieldset class="grid grid-cols-3 gap-3 @min-[44rem]:grid-cols-9">
    <legend class="sr-only">{m('settings.preferences.accent.title')}</legend>
    {#each accentColors as color (color)}
      <label
        class="group/accent flex min-w-0 cursor-pointer flex-col gap-2 rounded-lg"
        data-accent={color}
      >
        <input
          type="radio"
          name={groupName}
          value={color}
          checked={value === color}
          onchange={() => onchange(color)}
          class="peer sr-only"
        />
        <span
          class={[
            'relative flex h-20 items-end justify-end overflow-hidden rounded-lg accent-swatch shimmer-hover p-2 transition-transform duration-150 group-hover/accent:-translate-y-0.5 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-text motion-reduce:transform-none motion-reduce:before:animate-none',
            value === color && 'ring-2 ring-text ring-offset-2 ring-offset-background'
          ]}
          aria-hidden="true"
        >
          {#if value === color}
            <span
              class="flex size-6 items-center justify-center rounded-full bg-white text-black shadow-sm"
            >
              <span class="iconify icon-[uil--check] text-base"></span>
            </span>
          {/if}
        </span>
        <span
          class={[
            'max-w-full text-center break-words',
            value === color ? 'font-semibold text-text-top' : 'text-muted'
          ]}>{m(`settings.preferences.accent.${color}`)}</span
        >
      </label>
    {/each}
  </fieldset>

  <div
    class="mt-5 flex flex-wrap items-center justify-between gap-4 rounded-lg bg-surface px-4 py-3"
  >
    <span class="text-muted">{m('settings.preferences.accent.preview')}</span>
    <div
      role="img"
      aria-label={m('settings.preferences.accent.preview')}
      class="flex min-w-0 flex-wrap items-center justify-end gap-4"
    >
      <span
        class="flex h-6 w-10 items-center justify-end rounded-full bg-action p-1"
        aria-hidden="true"
      >
        <span class="size-4 rounded-full bg-on-action shadow-sm"></span>
      </span>
      <span class="font-medium text-action underline underline-offset-4" aria-hidden="true"
        >{m(`settings.preferences.accent.${value}`)}</span
      >
      <span class="pointer-events-none btn-action" aria-hidden="true">
        {m('common.continue')}
        <span class="iconify icon-[uil--arrow-right] rtl:-scale-x-100"></span>
      </span>
    </div>
  </div>
</div>
