<script lang="ts">
  import AnsiText from "./AnsiText.svelte";
  import { ansiTokens } from "$lib/ansi.ts";
  let { value, kind }: { value: unknown; kind: "input" | "output" | "logs" } = $props();
  let title = $derived(kind === "input" ? "Input" : kind === "logs" ? "Logs" : "Result");
  let wrap = $state(true);
  let copyState = $state<"idle" | "copied" | "failed">("idle");
  let text = $derived(
    typeof value === "string" ? value : JSON.stringify(value, null, 2) ?? "",
  );

  async function copy() {
    try {
      await navigator.clipboard.writeText(
        kind === "logs" ? ansiTokens(text).map((token) => token.text).join("") : text,
      );
      copyState = "copied";
    } catch {
      copyState = "failed";
    }
  }
</script>

<section class="overflow-hidden rounded-lg border border-base-300" aria-label={`Workflow ${kind}`}>
  <header class="flex flex-wrap items-center justify-between gap-2 border-b border-base-300 bg-base-200/50 px-4 py-2">
    <div class="flex items-center gap-2">
      <span class="icon-[lucide--file-text] size-4 text-base-content/50" aria-hidden="true"></span>
      <h2 class="text-sm font-medium">{title}</h2>
      <span class="badge badge-ghost badge-sm text-xs">{typeof value === "string" ? "Text" : "JSON"}</span>
    </div>
    <div class="flex items-center gap-1">
      <button class="btn btn-ghost btn-sm cursor-pointer gap-1.5" aria-pressed={wrap} onclick={() => (wrap = !wrap)}>
        <span class="icon-[lucide--wrap-text] size-4" aria-hidden="true"></span>
        Wrap
      </button>
      <button class="btn btn-ghost btn-sm cursor-pointer gap-1.5" onclick={copy}>
        <span class={copyState === "copied" ? "icon-[lucide--check] size-4" : "icon-[lucide--copy] size-4"} aria-hidden="true"></span>
        {copyState === "copied" ? "Copied" : `Copy ${kind}`}
      </button>
    </div>
  </header>
  <div class="max-h-[70vh] overflow-auto p-5">
    {#if text === ""}
      <p class="text-sm text-base-content/60">{kind === "logs" ? "No logs yet." : kind === "input" ? "The workflow received an empty string." : "The workflow returned an empty string."}</p>
    {:else}
      <pre class="m-0 font-mono text-sm leading-7 {wrap ? 'whitespace-pre-wrap wrap-anywhere' : 'whitespace-pre'}">{#if kind === "logs"}<AnsiText {text} />{:else}{text}{/if}</pre>
    {/if}
  </div>
  <p role="status" class="sr-only">{copyState === "copied" ? `${kind === "logs" ? "Logs" : kind === "input" ? "Input" : "Output"} copied to clipboard.` : ""}</p>
  {#if copyState === "failed"}
    <p role="alert" class="border-t border-base-300 px-4 py-2 text-xs text-error">Could not copy to the clipboard. Select the {kind} and copy it manually.</p>
  {/if}
</section>
