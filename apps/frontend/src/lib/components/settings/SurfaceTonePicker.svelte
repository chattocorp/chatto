<!--
@component
Curated surface tones for one theme, with native radio keyboard behaviour and a
visible check. The owner shows the picker for the active theme only. Each sample scopes its own `data-tone` and draws a small app
mock-up in the steps that theme uses, whatever theme is currently active.
The owner applies and saves changes.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import {
    surfaceTones,
    type EffectiveTheme,
    type SurfaceTone
  } from '$lib/state/userPreferences.svelte';

  let {
    theme,
    value,
    onchange
  }: {
    /** The theme whose tone this picker selects; samples always render in it. */
    theme: EffectiveTheme;
    value: SurfaceTone;
    onchange: (value: SurfaceTone) => void;
  } = $props();
  const groupName = $props.id();

  // Mirror the base contrast tokens in app.css for each theme.
  const sample = $derived(
    theme === 'dark'
      ? {
          background: 'bg-(--tone-900)',
          surface: 'bg-(--tone-800)',
          heading: 'bg-white',
          text: 'bg-(--tone-300)',
          muted: 'bg-(--tone-500)',
          accent: 'bg-(--accent-bright)'
        }
      : {
          background: 'bg-(--tone-100)',
          surface: 'bg-(--tone-200)',
          heading: 'bg-black',
          text: 'bg-(--tone-600)',
          muted: 'bg-(--tone-400)',
          accent: 'bg-(--accent-strong)'
        }
  );
</script>

<div class="@container">
  <fieldset class="grid grid-cols-3 gap-3 @min-[36rem]:grid-cols-6" data-tone-theme={theme}>
    <legend class="sr-only">{m('settings.preferences.tone.title')}</legend>
    {#each surfaceTones as tone (tone)}
      <label
        class="group/tone relative flex min-w-0 cursor-pointer flex-col gap-2 rounded-lg"
        data-tone={tone}
      >
        <input
          type="radio"
          name={groupName}
          value={tone}
          checked={value === tone}
          onchange={() => onchange(tone)}
          class="peer sr-only start-1 top-1"
        />
        <span
          class={[
            'relative flex h-16 overflow-hidden rounded-lg border transition-transform duration-150 group-hover/tone:-translate-y-0.5 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-text motion-reduce:transform-none',
            sample.background,
            theme === 'dark' ? 'border-white/10' : 'border-black/10',
            value === tone && 'ring-2 ring-text ring-offset-2 ring-offset-background'
          ]}
          aria-hidden="true"
        >
          <span class={['w-1/4 shrink-0', sample.surface]}></span>
          <span class="flex min-w-0 flex-1 flex-col justify-center gap-1.5 px-2">
            <span class={['h-1.5 w-1/2 rounded-full', sample.heading]}></span>
            <span class={['h-1.5 w-4/5 rounded-full', sample.text]}></span>
            <span class={['h-1.5 w-3/5 rounded-full', sample.muted]}></span>
          </span>
          <span class={['absolute end-2 top-2 size-2 rounded-full', sample.accent]}></span>
          {#if value === tone}
            <span
              class="absolute end-1.5 bottom-1.5 flex size-5 items-center justify-center rounded-full bg-white text-black shadow-sm"
            >
              <span class="iconify icon-[uil--check] text-sm"></span>
            </span>
          {/if}
        </span>
        <span
          class={[
            'max-w-full text-center break-words',
            value === tone ? 'font-semibold text-text-top' : 'text-muted'
          ]}>{m(`settings.preferences.tone.${tone}`)}</span
        >
      </label>
    {/each}
  </fieldset>
</div>
