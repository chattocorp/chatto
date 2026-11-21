<!--
@component
One self-verified identity on a profile: the method icon, the identity, and a
green check. Only the check is colored, so the row reads as profile information.
The identity stays on one line and ends with an ellipsis when space is limited.
The full identity and method are available on hover and to screen readers.
-->
<script lang="ts">
  let { method, icon, identity }: { method: string; icon: string; identity: string } = $props();
</script>

<span
  title={`${method} · ${identity}`}
  class="inline-flex max-w-full items-center gap-1.5 align-middle text-sm leading-5"
>
  <span class="sr-only">{method} </span>
  <span aria-hidden="true" class={['iconify size-4 shrink-0 text-muted', icon]}></span>
  <bdi dir="auto" class="block min-w-0 truncate">{identity}</bdi>
  <span aria-hidden="true" class="iconify icon-[mdi--check-decagram] size-4 shrink-0 text-success"
  ></span>
</span>
