<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { csrfFetch } from '$lib/auth/csrf';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import * as m from '$lib/i18n/messages';
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
        window.location.href =
          resolve('/login') + `?redirect=${encodeURIComponent('/oauth/consent')}`;
        return;
      }

      const result = await response.json();
      if (!response.ok) {
        error = result.error || m['auth.oauth.request_not_found']();
        return;
      }

      const pendingRequest = {
        redirectUri: result.redirectUri,
        redirectOrigin: result.redirectOrigin
      };
      const verifiedHost = verifiedRequesterHost(pendingRequest);
      if (!verifiedHost) {
        error = m['auth.oauth.unverifiable']();
        return;
      }

      requesterHost = verifiedHost;
      request = pendingRequest;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = m['auth.oauth.request_timeout']();
      } else {
        error = err instanceof Error ? err.message : m['auth.oauth.request_load_failed']();
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
        error = result.error || m['auth.oauth.submit_failed']();
        return;
      }
      if (!result.redirectUrl) {
        error = m['auth.oauth.missing_redirect']();
        return;
      }

      window.location.href = result.redirectUrl;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = m['auth.oauth.decision_timeout']();
      } else {
        error = err instanceof Error ? err.message : m['auth.oauth.submit_failed']();
      }
    } finally {
      submitting = null;
    }
  }
</script>

<PageTitle title={m['auth.oauth.title']()} />

<AuthLayout>
  <div class="flex flex-col gap-6">
    <div class="text-center">
      <div
        class="mb-4 inline-flex h-12 w-12 items-center justify-center rounded-full bg-accent/10 text-accent"
      >
        <span class="iconify text-2xl mdi--shield-check"></span>
      </div>
      <h1 class="text-2xl font-bold">{m['auth.oauth.heading']()}</h1>
    </div>

    {#if loading}
      <div class="flex justify-center py-8">
        <span class="iconify animate-spin text-3xl text-muted mdi--loading"></span>
      </div>
    {:else if request}
      <div class="flex flex-col gap-5">
        <div class="text-center">
          <p class="text-base leading-relaxed text-muted">{m['auth.oauth.requester_intro']()}</p>
          <p class="mt-1 text-base font-semibold">{requesterHost}</p>
        </div>

        <div class="surface-box p-4">
          <div class="mb-3 text-sm font-medium">{m['auth.oauth.allow_intro']()}</div>
          <ul class="flex flex-col gap-2 text-sm text-muted">
            <li class="flex gap-2">
              <span class="mt-0.5 iconify shrink-0 text-accent mdi--check"></span>
              <span>{m['auth.oauth.allow_profile']()}</span>
            </li>
            <li class="flex gap-2">
              <span class="mt-0.5 iconify shrink-0 text-accent mdi--check"></span>
              <span>{m['auth.oauth.allow_messages']()}</span>
            </li>
            <li class="flex gap-2">
              <span class="mt-0.5 iconify shrink-0 text-accent mdi--check"></span>
              <span>{m['auth.oauth.allow_remember']()}</span>
            </li>
          </ul>
        </div>

        <FormError {error} />

        <div class="flex flex-col gap-3">
          <Button
            size="lg"
            fullWidth
            loading={submitting === 'approve'}
            loadingText={m['auth.oauth.authorizing']()}
            disabled={submitting !== null}
            onclick={() => submitConsent('approve')}
          >
            <span class="iconify mdi--check"></span>
            {m['auth.oauth.title']()}
          </Button>
          <Button
            variant="secondary"
            size="lg"
            fullWidth
            loading={submitting === 'deny'}
            loadingText={m['auth.oauth.denying']()}
            disabled={submitting !== null}
            onclick={() => submitConsent('deny')}
          >
            <span class="iconify mdi--close"></span>
            {m['common.cancel']()}
          </Button>
        </div>
      </div>
    {:else}
      <div class="flex flex-col gap-4 text-center">
        <FormError {error} />
        <Button variant="secondary" size="lg" fullWidth onclick={() => goto(resolve('/'))}>
          {m['auth.oauth.return_home']()}
        </Button>
      </div>
    {/if}
  </div>
</AuthLayout>
