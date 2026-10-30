<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Foundations/Hover Lift',
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Hover Lift raises an interactive surface by 2 px over 150 ms. It uses the shared hover-lift utility, requires pointer hover support, and follows the app motion preference. Keyboard focus keeps its outline without movement.'
        }
      }
    }
  });
</script>

<Story name="Interactive card" asChild>
  <div class="p-4">
    <button
      type="button"
      class="flex w-64 hover-lift cursor-pointer flex-col gap-2 surface-box p-6 text-start focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-action"
    >
      <span class="font-bold text-text-top">Settings Overview</span>
      <span class="text-muted">Hover anywhere on this card to lift the whole surface.</span>
    </button>
  </div>
</Story>

<Story name="Labelled visual choice" asChild>
  <div class="p-4">
    <label class="hover-lift-group relative flex w-48 cursor-pointer flex-col gap-3">
      <input type="radio" name="hover-lift-example" class="peer sr-only" checked />
      <span
        class="flex h-24 hover-lift items-center justify-center rounded-lg border border-border bg-action/10 text-action peer-focus-visible:outline-2 peer-focus-visible:outline-offset-4 peer-focus-visible:outline-action"
      >
        <span aria-hidden="true" class="iconify icon-[uil--palette] text-3xl"></span>
      </span>
      <span class="font-bold text-text-top">Appearance preview</span>
      <span class="text-muted">The label triggers the lift. Only the preview moves.</span>
    </label>
  </div>
</Story>
