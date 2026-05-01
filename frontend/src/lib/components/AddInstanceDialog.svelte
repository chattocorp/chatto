<!--
@component

The "Add Instance" form as a modal. Collects a URL, probes
`/api/instance`, and on success closes the dialog and navigates to
`/instances/add/[hostname]` for the auth-method picker (a separate
full-page step that needs the OAuth redirect surface).
-->
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { TextInput } from '$lib/ui/form';
  import FormDialog from '$lib/ui/FormDialog.svelte';

  let {
    visible = $bindable(false),
    onclose
  }: {
    visible?: boolean;
    onclose: () => void;
  } = $props();

  let instanceUrl = $state('');
  let urlError = $state('');
  let probing = $state(false);

  // Reset form state whenever the dialog is closed so reopening starts
  // fresh — otherwise the previously typed URL and any prior error
  // message would still be visible on the next open. The component is
  // mounted persistently by its callers (sidebar, instances page, login),
  // so without this the state survives across open/close cycles.
  $effect(() => {
    if (!visible) {
      instanceUrl = '';
      urlError = '';
    }
  });

  function normalizeUrl(url: string): string {
    let u = url.trim().replace(/\/+$/, '');
    if (!/^https?:\/\//i.test(u)) {
      u = 'https://' + u;
    }
    try {
      return new URL(u).origin;
    } catch {
      return u;
    }
  }

  async function handleSubmit() {
    urlError = '';

    const url = normalizeUrl(instanceUrl);

    try {
      new URL(url);
    } catch {
      urlError = 'Please enter a valid URL.';
      return;
    }

    const existing = instanceRegistry.instances.find(
      (i) => i.url.toLowerCase() === url.toLowerCase()
    );
    if (existing && (existing.token || existing.userId)) {
      urlError = 'This instance is already connected.';
      return;
    }

    probing = true;

    try {
      const response = await fetch(`${url}/api/instance`, {
        signal: AbortSignal.timeout(10000)
      });

      if (!response.ok) {
        urlError = `Server responded with ${response.status}. Is this a Chatto instance?`;
        return;
      }

      const info = await response.json();

      if (!info.name || !Array.isArray(info.authMethods)) {
        urlError = 'This does not appear to be a Chatto instance.';
        return;
      }

      if (!info.authorizeUrl) {
        urlError = 'This instance does not support OAuth authentication. It may need to be updated.';
        return;
      }

      const hostname = new URL(url).host;
      onclose();
      goto(resolve('/instances/add/[hostname]', { hostname }));
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        urlError = 'Connection timed out. Check the URL and try again.';
      } else if (err instanceof TypeError) {
        urlError = 'Could not connect. Check the URL and ensure CORS is configured.';
      } else {
        urlError = err instanceof Error ? err.message : 'Failed to connect.';
      }
    } finally {
      probing = false;
    }
  }
</script>

<FormDialog
  bind:visible
  title="Add Instance"
  size="sm"
  submitLabel="Connect"
  submitIcon="iconify uil--link"
  submitLoadingText="Connecting…"
  loading={probing}
  disabled={!instanceUrl.trim()}
  error={urlError}
  onsubmit={handleSubmit}
  {onclose}
>
  {#snippet description()}
    Chatto is a distributed chat platform — your client connects to each
    instance directly. Enter a URL to add another one.
  {/snippet}

  <TextInput
    id="add-instance-url"
    label="Instance URL"
    bind:value={instanceUrl}
    placeholder="chat.example.com"
    leadingIcon="uil--globe"
    disabled={probing}
    required
    autofocus
  />
</FormDialog>
