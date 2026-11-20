<!--
@component
A profile label for one self-verification method. The icon identifies the method;
its name is also available on hover and to screen readers.
-->
<script lang="ts">
  import { Pill } from '$lib/ui';

  let { method, icon, identity }: { method: string; icon: string; identity: string } = $props();
</script>

<Pill
  tone="success"
  title={method}
  paddingClass="ps-1.5 pe-2.5 py-0.5"
  class="max-w-full rounded-full ring-1 ring-success/20 ring-inset"
>
  <span class="flex min-w-0 items-center gap-1.5 leading-4">
    <span class="sr-only">{method} </span>
    <span aria-hidden="true" class={['iconify size-3.5 shrink-0', icon]}></span>
    <span aria-hidden="true" class="h-3 w-px shrink-0 bg-success/35"></span>
    <bdi dir="auto" class="min-w-0 wrap-anywhere">{identity}</bdi>
  </span>
</Pill>
