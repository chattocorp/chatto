<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { TextInput, FormError, Button } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  // Redirect to login if origin exists but user isn't authenticated
  const origin = $derived(instanceRegistry.originInstance);
  const originAuthenticated = $derived(origin ? instanceRegistry.isAuthenticated(origin.id) : false);
  $effect(() => {
    if (origin && !originAuthenticated) {
      goto(resolve('/login'), { replaceState: true });
    }
  });

  // --- Remote instance URL form ---
  let instanceUrl = $state('');
  let urlError = $state('');
  let probing = $state(false);

  function normalizeUrl(url: string): string {
    let u = url.trim().replace(/\/+$/, '');
    if (!/^https?:\/\//i.test(u)) {
      u = 'https://' + u;
    }
    try {
      const parsed = new URL(u);
      return parsed.origin;
    } catch {
      return u;
    }
  }

  async function handleUrlSubmit(e: Event) {
    e.preventDefault();
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

<PageTitle title="Add Instance" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Add Instance" showMobileNav />

  <div class="flex-1 overflow-y-auto">
    <section class="mx-auto w-full max-w-sm p-6">
      <div class="flex flex-col gap-4">
        <div>
          <h3 class="text-lg font-semibold">Connect to a remote instance</h3>
          <p class="text-sm text-muted">
            Chatto is a distributed chat platform made up of many separate instances
            located all over the world. Instances don't talk to each other — your
            client connects to each one directly.
          </p>
          <p class="text-sm text-muted">
            Enter a URL to add another instance to this client.
          </p>
        </div>

        <form onsubmit={handleUrlSubmit} class="flex flex-col gap-4">
          <TextInput
            id="instance-url"
            label="Instance URL"
            type="text"
            bind:value={instanceUrl}
            placeholder="chat.example.com"
            disabled={probing}
            required
            autofocus
          />

          <FormError error={urlError} />

          <Button type="submit" disabled={!instanceUrl.trim() || probing} loading={probing} loadingText="Connecting...">
            Connect
          </Button>
        </form>
      </div>
    </section>
  </div>
</div>
