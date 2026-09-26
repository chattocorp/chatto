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
        class="group/accent relative flex min-w-0 cursor-pointer flex-col gap-2 rounded-lg"
        data-accent={color}
      >
        <!-- Keep the native focus target inside its swatch when the page scrolls. -->
        <input
          type="radio"
          name={groupName}
          value={color}
          checked={value === color}
          onchange={() => onchange(color)}
          class="peer sr-only start-1 top-1"
        />
        <span
          class={[
            'shimmer-hover relative flex h-20 items-end justify-end overflow-hidden rounded-lg accent-swatch p-2 transition-transform duration-150 group-hover/accent:-translate-y-0.5 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-text motion-reduce:transform-none motion-reduce:before:animate-none',
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
</div>
