<script lang="ts">
  import { submitFormShortcut } from '$lib/form-shortcut.ts';

  let { runId }: { runId: string } = $props();
  let text = $state('');
  let pending = $state(false);
  let error = $state('');
  const errorId = $props.id();

  /** Send JSON text as its value. Send all other text as a string. */
  function parse(value: string): unknown {
    try {
      return JSON.parse(value);
    } catch {
      return value;
    }
  }

  async function send(event: SubmitEvent) {
    event.preventDefault();
    if (pending || !text.trim()) return;
    pending = true;
    error = '';
    try {
      const response = await fetch(`/api/runs/${runId}/messages`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ message: parse(text) })
      });
      if (!response.ok) {
        const result = await response.json().catch(() => ({}));
        throw new Error(result.error ?? 'Could not send the message.');
      }
      text = '';
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Could not send the message.';
    } finally {
      pending = false;
    }
  }
</script>

<form
  class="shrink-0 border-t border-base-300 px-4 py-3 sm:px-6"
  aria-label="Send a message to the workflow"
  onsubmit={send}
>
  <div class="flex items-end gap-2">
    <textarea
      class="textarea min-h-10 flex-1 resize-y font-mono text-sm leading-relaxed"
      rows="1"
      placeholder="Message to the workflow. Text or JSON."
      aria-label="Message to the workflow"
      aria-describedby={error ? errorId : undefined}
      spellcheck="false"
      bind:value={text}
      onkeydown={(event) => submitFormShortcut(event, pending, event.currentTarget.form!)}
    ></textarea>
    <button
      class="btn btn-sm btn-primary"
      disabled={pending || !text.trim()}
      aria-keyshortcuts="Meta+Enter Control+Enter"
      title="Send (Cmd/Ctrl+Enter)">{pending ? 'Sending…' : 'Send'}</button
    >
  </div>
  {#if error}<p id={errorId} class="mt-2 text-sm text-error" role="alert">{error}</p>{/if}
</form>
