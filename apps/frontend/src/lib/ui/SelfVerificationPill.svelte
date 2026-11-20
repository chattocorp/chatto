<!--
@component
A profile label for one self-verification method. The icon identifies the method;
the identity stays on one line and ends with an ellipsis when space is limited.
The full identity and method are available on hover and to screen readers.
-->
<script lang="ts">
  let { method, icon, identity }: { method: string; icon: string; identity: string } = $props();
</script>

<span
  title={`${method} · ${identity}`}
  class="inline-flex max-w-full items-stretch gap-px overflow-hidden rounded-full align-middle text-xs leading-4 font-medium text-success"
>
  <span class="grid shrink-0 place-items-center bg-success/20 py-0.5 ps-1.5 pe-1">
    <span class="sr-only">{method} </span>
    <span aria-hidden="true" class={['iconify size-3.5', icon]}></span>
  </span>
  <span class="min-w-0 bg-success/20 py-0.5 ps-1.5 pe-2.5">
    <bdi dir="auto" class="block truncate">{identity}</bdi>
  </span>
</span>
