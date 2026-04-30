<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Checkbox from './Checkbox.svelte';

  const { Story } = defineMeta({
    title: 'Form/Checkbox',
    component: Checkbox,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let a = $state(false);
  let b = $state(true);
  let c = $state(false);
  let d = $state(false);
</script>

<Story name="Default" asChild>
  <Checkbox id="a" bind:checked={a} label="Send me email notifications" />
</Story>

<Story name="Pre-checked" asChild>
  <Checkbox id="b" bind:checked={b} label="Public room (visible in listings)" />
</Story>

<Story name="With error" asChild>
  <Checkbox
    id="c"
    bind:checked={c}
    label="I accept the terms"
    error="You must accept the terms to continue."
  />
</Story>

<Story name="Disabled" asChild>
  <Checkbox id="d" bind:checked={d} label="Disabled option" disabled />
</Story>

<Story name="Group" asChild>
  <div class="flex flex-col gap-2">
    <Checkbox id="g1" bind:checked={a} label="Mentions" />
    <Checkbox id="g2" bind:checked={b} label="Direct messages" />
    <Checkbox id="g3" bind:checked={c} label="Replies in my threads" />
    <Checkbox id="g4" bind:checked={d} label="All messages in subscribed rooms" />
  </div>
</Story>
