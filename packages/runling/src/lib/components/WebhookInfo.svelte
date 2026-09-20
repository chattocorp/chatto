<script lang="ts">
  import { onMount } from "svelte";
  import type { WebhookInfo } from "$lib/runs.ts";
  import { webhookCurl } from "$lib/webhook-curl.ts";

  let { webhook, onclose }: { webhook: WebhookInfo; onclose: () => void } =
    $props();
  let dialog: HTMLDialogElement;
  let origin = $state("");
  let message = $state("");
  let url = $derived(`${origin}${webhook.path}`);
  let curl = $derived(webhookCurl(url, webhook.input));
  onMount(() => {
    origin = location.origin;
    dialog.showModal();
  });
  async function copy(value: string, label: string) {
    try {
      await navigator.clipboard.writeText(value);
      message = `${label} copied.`;
    } catch {
      message =
        "Could not access the clipboard. Select and copy the text below.";
    }
  }
</script>

<dialog
  class="modal modal-middle"
  bind:this={dialog}
  {onclose}
  aria-labelledby="hook-info-title"
>
  <div class="modal-box w-11/12 max-w-2xl p-0">
    <header class="flex items-center justify-between gap-4 px-6 pt-6 pb-4">
      <div>
        <p class="text-base-content/60 text-xs mt-0 mr-0 mb-2 ml-0">
          Configured webhook
        </p>
        <h2 class="m-0 text-2xl font-medium wrap-anywhere" id="hook-info-title">
          {webhook.name}
        </h2>
      </div>
      <button
        class="btn btn-ghost btn-sm"
        onclick={() => dialog.close()}
        aria-label="Close webhook information">×</button
      >
    </header>
    <div class="px-6 pb-6">
      <dl
        class="grid grid-cols-[auto_minmax(0,_1fr)] gap-y-2.5 gap-x-5 text-xs"
      >
        <dt class="text-base-content/60">Route</dt>
        <dd class="m-0 wrap-anywhere">{webhook.workflow}</dd>
        <dt class="text-base-content/60">Method</dt>
        <dd class="m-0 wrap-anywhere"><code class="font-mono">POST</code></dd>
        <dt class="text-base-content/60">Content type</dt>
        <dd class="m-0 wrap-anywhere">
          <code class="font-mono">application/json</code>
        </dd>
      </dl>
      <p class="text-base-content/60 text-xs leading-relaxed">
        The router handles the request and can start one or more runs. The response
        lists their IDs without waiting for completion. An empty list means no
        new run was started.
      </p>
      <section class="mt-6" aria-labelledby="hook-url-title">
        <div class="flex items-center justify-between gap-4">
          <h3 class="text-sm font-medium m-0" id="hook-url-title">
            Webhook URL
          </h3>
          <button
            class="btn btn-ghost btn-sm shrink-0 text-xs"
            onclick={() => copy(url, "URL")}>Copy URL</button
          >
        </div>
        <pre
          class="font-mono bg-base-200 border border-base-300 rounded-md p-3.5 text-xs leading-relaxed whitespace-pre-wrap wrap-anywhere max-h-70 overflow-auto">{url}</pre>
      </section>
      <section class="mt-6" aria-labelledby="hook-curl-title">
        <div class="flex items-center justify-between gap-4">
          <h3 class="text-sm font-medium m-0" id="hook-curl-title">
            Example curl command
          </h3>
          <button
            class="btn btn-ghost btn-sm shrink-0 text-xs"
            onclick={() => copy(curl, "Command")}>Copy curl</button
          >
        </div>
        <p class="text-base-content/60 text-xs leading-relaxed">
          For bash or zsh. This sample comes from the webhook input schema.
          Check its values and add any required authentication headers before
          use.
        </p>
        <pre
          class="font-mono bg-base-200 border border-base-300 rounded-md p-3.5 text-xs leading-relaxed whitespace-pre-wrap wrap-anywhere max-h-70 overflow-auto">{curl}</pre>
      </section>
      <p class="min-h-5 text-xs text-base-content/60" role="status">
        {message}
      </p>
      <section class="mt-6" aria-labelledby="hook-schemas-title">
        <h3 class="text-sm font-medium m-0" id="hook-schemas-title">Schemas</h3>
        <details
          class="collapse collapse-arrow border border-base-300 bg-base-200 mt-3"
        >
          <summary class="collapse-title text-sm font-medium"
            >Webhook input</summary
          >
          <div class="collapse-content">
            <pre
              class="font-mono bg-base-200 border border-base-300 rounded-md p-3.5 text-xs leading-relaxed whitespace-pre-wrap wrap-anywhere max-h-70 overflow-auto">{JSON.stringify(
                webhook.input,
                null,
                2,
              )}</pre>
          </div>
        </details>
        <details
          class="collapse collapse-arrow border border-base-300 bg-base-200 mt-3"
        >
          <summary class="collapse-title text-sm font-medium"
            >Workflow output (when declared)</summary
          >
          <div class="collapse-content">
            <pre
              class="font-mono bg-base-200 border border-base-300 rounded-md p-3.5 text-xs leading-relaxed whitespace-pre-wrap wrap-anywhere max-h-70 overflow-auto">{JSON.stringify(
                webhook.output,
                null,
                2,
              )}</pre>
          </div>
        </details>
      </section>
    </div>
  </div>
</dialog>
