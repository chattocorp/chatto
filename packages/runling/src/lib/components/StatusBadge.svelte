<script lang="ts">
  let { status, waiting = false }: { status: string; waiting?: boolean } = $props();
  let waitingForInput = $derived(status === "running" && waiting);
  let label = $derived(waitingForInput ? "Inputs pending" : status.charAt(0).toUpperCase() + status.slice(1));
  let icon = $derived(
    waitingForInput
      ? "icon-[lucide--message-circle-question] text-warning"
      : status === "completed"
      ? "icon-[lucide--circle-check] text-success"
      : status === "running"
        ? "icon-[lucide--loader-circle] text-info animate-spin motion-reduce:animate-none"
        : status === "failed"
          ? "icon-[lucide--circle-x] text-error"
          : status === "cancelled"
            ? "icon-[lucide--circle-stop] text-warning"
          : status === "interrupted"
            ? "icon-[lucide--circle-pause] text-warning"
            : status === "blocked"
              ? "icon-[lucide--ban] text-error"
              : "icon-[lucide--circle-help] text-base-content/60",
  );
</script>

<span
  class={["inline-block size-4 shrink-0 align-middle", icon]}
  role="img"
  aria-label={label}
  title={label}
></span>
