<script lang="ts">
  import { onMount } from 'svelte';
  import type { WebhookInfo } from '$lib/runs.ts';
  import { sample } from '$lib/sample.ts';
  import { submitFormShortcut } from '$lib/form-shortcut.ts';
  let {
    webhooks,
    initialWebhook,
    onclose,
    onstarted
  }: {
    webhooks: WebhookInfo[];
    initialWebhook: string;
    onclose: () => void;
    onstarted: (id: string) => void;
  } = $props();
  let webhookName = $state('');
  let webhook = $derived(webhooks.find((item) => item.name === webhookName) ?? webhooks[0]!);
  let dialog: HTMLDialogElement;
  let form: HTMLFormElement;
  let body = $state('');
  let pending = $state(false);
  let error = $state('');
  let notice = $state('');
  let startedRuns = $state<{ id: string }[]>([]);
  let schemaTab = $state<'input' | 'output'>('input');
  let origin = $state('');
  let copied = $state(false);
  onMount(() => {
    const initial = webhooks.find((item) => item.name === initialWebhook) ?? webhooks[0]!;
    webhookName = initial.name;
    body = JSON.stringify(sample(initial.input), null, 2);
    origin = location.origin;
    dialog.showModal();
  });
  function selectWebhook(name: string) {
    webhookName = name;
    const selected = webhooks.find((item) => item.name === name);
    if (!selected) return;
    body = JSON.stringify(sample(selected.input), null, 2);
    error = '';
    notice = '';
    startedRuns = [];
    copied = false;
  }
  async function start(event: SubmitEvent) {
    event.preventDefault();
    if (pending) return;
    error = '';
    notice = '';
    startedRuns = [];
    try {
      JSON.parse(body);
    } catch {
      error = 'Enter valid JSON before starting the run.';
      return;
    }
    pending = true;
    try {
      const response = await fetch(`/api/runs/start/${encodeURIComponent(webhook.name)}`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body
      });
      const result = await response.json();
      startedRuns = result.runs ?? [];
      if (!response.ok)
        throw new Error(
          [
            result.error ?? 'Could not start the run.',
            ...(result.issues ?? []).map(
              (issue: { path: string; message: string }) => `${issue.path}: ${issue.message}`
            )
          ].join('\n')
        );
      if (!startedRuns.length) {
        notice = 'Message handled. No new run was started.';
        return;
      }
      if (startedRuns.length > 1) {
        notice = `Started ${startedRuns.length} runs.`;
        return;
      }
      onstarted(startedRuns[0]!.id);
      dialog.close();
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      pending = false;
    }
  }
  async function copy() {
    try {
      await navigator.clipboard.writeText(`${origin}${webhook.path}`);
      copied = true;
    } catch {
      error = 'Could not copy the URL. Select and copy it from the address below.';
    }
  }
  function handleShortcut(event: KeyboardEvent) {
    if (dialog?.contains(event.target as Node)) submitFormShortcut(event, pending, form);
  }
</script>

<svelte:window onkeydown={handleShortcut} />

<dialog class="modal modal-middle" bind:this={dialog} {onclose} aria-labelledby="composer-title">
  <div class="modal-box w-11/12 max-w-2xl p-0">
    <div class="flex justify-between items-center px-6 pt-6 pb-4">
      <div>
        <h2 class="m-0 text-2xl font-medium" id="composer-title">New run</h2>
      </div>
      <button class="btn btn-ghost btn-sm" onclick={() => dialog.close()} aria-label="Close new run"
        >×</button
      >
    </div>
    <div class="mx-6 mb-4">
      <label class="label mb-1 text-sm font-medium" for="new-run-webhook">Webhook</label>
      <select
        id="new-run-webhook"
        class="select w-full"
        value={webhookName}
        disabled={pending}
        onchange={(event) => selectWebhook(event.currentTarget.value)}
      >
        {#each webhooks as item (item.name)}<option value={item.name}
            >{item.workflow} ({item.name})</option
          >{/each}
      </select>
    </div>
    <div class="mx-6 flex items-center gap-2.5 bg-base-200 p-3 rounded-md">
      <code class="font-mono flex-1 wrap-anywhere text-xs">{origin}{webhook.path}</code><button
        class="btn btn-ghost btn-sm text-xs shrink-0"
        onclick={copy}>{copied ? 'Copied' : 'Copy URL'}</button
      >
    </div>
    <form class="p-6" bind:this={form} onsubmit={start}>
      <label class="label mb-2 font-medium text-sm" for="request-body">Request body</label>
      <p class="text-xs text-base-content/60 leading-relaxed">
        Edit the sample request. The schema below defines the accepted values.
      </p>
      <textarea
        class="textarea w-full min-h-48 font-mono text-sm leading-relaxed resize-y"
        id="request-body"
        bind:value={body}
        spellcheck="false"
        rows="8"
        aria-describedby={error ? 'request-error' : undefined}
      ></textarea>
      <details class="collapse collapse-arrow border border-base-300 bg-base-200 mt-3">
        <summary class="collapse-title text-sm font-medium">Inspect schemas</summary>
        <div class="collapse-content">
          <div class="flex gap-2 my-3.5 mx-0 flex-wrap">
            <button
              class="btn btn-sm btn-ghost"
              type="button"
              class:btn-active={schemaTab === 'input'}
              onclick={() => (schemaTab = 'input')}>Webhook input</button
            ><button
              class="btn btn-sm btn-ghost"
              type="button"
              class:btn-active={schemaTab === 'output'}
              onclick={() => (schemaTab = 'output')}>Workflow output (when declared)</button
            >
          </div>
          <pre class="font-mono max-h-56 overflow-auto text-xs leading-relaxed">{JSON.stringify(
              webhook[schemaTab],
              null,
              2
            )}</pre>
        </div>
      </details>
      {#if notice}<p class="alert alert-info" role="status">{notice}</p>{/if}
      {#if startedRuns.length}
        <ul class="mt-3 flex flex-wrap gap-2">
          {#each startedRuns as run (run.id)}
            <li>
              <button
                type="button"
                class="btn btn-sm btn-ghost"
                onclick={() => {
                  onstarted(run.id);
                  dialog.close();
                }}>Open run {run.id.slice(0, 8)}</button
              >
            </li>
          {/each}
        </ul>
      {/if}
      {#if error}<p id="request-error" class="alert alert-error" role="alert">
          {error}
        </p>{/if}
      <footer class="flex gap-5 items-center justify-between mt-6">
        <span class="text-xs text-base-content/60 leading-relaxed"
          >Cmd/Ctrl+Enter to start. The run continues if you close this window.</span
        ><button
          class="btn btn-sm btn-primary"
          disabled={pending}
          aria-keyshortcuts="Meta+Enter Control+Enter">{pending ? 'Starting…' : 'Start run'}</button
        >
      </footer>
    </form>
  </div>
</dialog>
