<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { csrfFetch } from '$lib/auth/csrf';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, FormError } from '$lib/ui/form';
  import { onMount } from 'svelte';

  type ConsentRequest = {
    redirectUri: string;
    redirectOrigin: string;
  };

  let request = $state<ConsentRequest | null>(null);
  let requesterHost = $state('');
  let error = $state('');
  let loading = $state(true);
  let submitting = $state<'approve' | 'deny' | null>(null);

  onMount(async () => {
    try {
      const response = await fetch('/oauth/consent/request', {
        credentials: 'include',
        signal: AbortSignal.timeout(10000)
      });

      if (response.status === 401) {
        window.location.href = resolve('/login') + `?redirect=${encodeURIComponent('/oauth/consent')}`;
        return;
      }

      const result = await response.json();
      if (!response.ok) {
        error = result.error || 'Authorization request not found.';
        return;
      }

      const pendingRequest = {
        redirectUri: result.redirectUri,
        redirectOrigin: result.redirectOrigin
      };
      const verifiedHost = verifiedRequesterHost(pendingRequest);
      if (!verifiedHost) {
        error = 'This authorization request could not be verified.';
        return;
      }

      requesterHost = verifiedHost;
      request = pendingRequest;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = 'Authorization request timed out. Please try again.';
      } else {
        error = err instanceof Error ? err.message : 'Failed to load authorization request.';
      }
    } finally {
      loading = false;
    }
  });

  function verifiedRequesterHost(pendingRequest: ConsentRequest) {
    try {
      const redirectUri = new URL(pendingRequest.redirectUri);
      const redirectOrigin = new URL(pendingRequest.redirectOrigin);
      if (
        redirectUri.protocol !== redirectOrigin.protocol ||
        redirectUri.hostname !== redirectOrigin.hostname ||
        redirectUri.port !== redirectOrigin.port
      ) {
        return '';
      }
      return redirectOrigin.host;
    } catch {
      return '';
    }
  }

  async function submitConsent(decision: 'approve' | 'deny') {
    error = '';
    submitting = decision;

    try {
      const response = await csrfFetch(`/oauth/consent/${decision}`, {
        method: 'POST',
        credentials: 'include',
        signal: AbortSignal.timeout(10000)
      });
      const result = await response.json();

      if (!response.ok) {
        error = result.error || 'Failed to submit authorization decision.';
        return;
      }
      if (!result.redirectUrl) {
        error = 'Authorization server did not return a redirect URL.';
        return;
      }

      window.location.href = result.redirectUrl;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = 'Authorization decision timed out. Please try again.';
      } else {
        error = err instanceof Error ? err.message : 'Failed to submit authorization decision.';
      }
    } finally {
      submitting = null;
    }
  }
</script>

<PageTitle title="Allow Access" />

<AuthLayout>
  <div class="flex flex-col gap-6">
    <div class="text-center">
      <div class="mb-4 inline-flex h-12 w-12 items-center justify-center rounded-full bg-accent/10 text-accent">
        <span class="iconify text-2xl mdi--shield-check"></span>
      </div>
      <h1 class="text-2xl font-bold">Allow Access?</h1>
    </div>

    {#if loading}
      <div class="flex justify-center py-8">
        <span class="iconify animate-spin text-3xl text-muted mdi--loading"></span>
      </div>
    {:else if request}
      <div class="flex flex-col gap-5">
        <div class="text-center">
          <p class="text-base leading-relaxed text-muted">
            <span class="font-semibold text-primary">{requesterHost}</span> is requesting access to your
            account.
          </p>
        </div>

        <div class="rounded-lg border border-border bg-surface-100 p-4">
          <div class="mb-3 text-sm font-medium">If you allow access:</div>
          <ul class="flex flex-col gap-2 text-sm text-muted">
            <li class="flex gap-2">
              <span class="iconify mt-0.5 shrink-0 text-accent mdi--check"></span>
              <span>It can see your profile and the server data available to you.</span>
            </li>
            <li class="flex gap-2">
              <span class="iconify mt-0.5 shrink-0 text-accent mdi--check"></span>
              <span>It can read and send messages as you.</span>
            </li>
            <li class="flex gap-2">
              <span class="iconify mt-0.5 shrink-0 text-accent mdi--check"></span>
              <span>Chatto will remember this approval for this address.</span>
            </li>
          </ul>
        </div>

        <FormError {error} />

        <div class="flex flex-col gap-3">
          <Button
            size="lg"
            fullWidth
            loading={submitting === 'approve'}
            loadingText="Authorizing..."
            disabled={submitting !== null}
            onclick={() => submitConsent('approve')}
          >
            <span class="iconify mdi--check"></span>
            Allow Access
          </Button>
          <Button
            variant="secondary"
            size="lg"
            fullWidth
            loading={submitting === 'deny'}
            loadingText="Denying..."
            disabled={submitting !== null}
            onclick={() => submitConsent('deny')}
          >
            <span class="iconify mdi--close"></span>
            Cancel
          </Button>
        </div>
      </div>
    {:else}
      <div class="flex flex-col gap-4 text-center">
        <FormError {error} />
        <Button variant="secondary" size="lg" fullWidth onclick={() => goto(resolve('/'))}>
          Return to Chatto
        </Button>
      </div>
    {/if}
  </div>
</AuthLayout>
