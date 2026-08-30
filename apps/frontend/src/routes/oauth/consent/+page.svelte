<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { csrfFetch } from '$lib/auth/csrf';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import Hint from '$lib/ui/Hint.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, FormError } from '$lib/ui/form';
  import { onMount } from 'svelte';

  type ConsentRequest = {
    redirectUri: string;
    redirectOrigin: string;
    localRedirect: boolean;
    clientId: string;
    clientName: string;
    clientUri: string;
    resource: string;
    scopes: string[];
  };

  let request = $state<ConsentRequest | null>(null);
  let clientHost = $state('');
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
        error = result.error || m('auth.oauth.request_not_found');
        return;
      }

      const pendingRequest = {
        redirectUri: result.redirectUri,
        redirectOrigin: result.redirectOrigin,
        localRedirect: isLocalCallback(result.redirectUri),
        clientId: result.clientId,
        clientName: result.clientName,
        clientUri: result.clientUri,
        resource: result.resource || '',
        scopes: Array.isArray(result.scopes) ? result.scopes : []
      };
      const verifiedHost = verifiedClientHost(pendingRequest);
      if (!verifiedHost) {
        error = m('auth.oauth.unverifiable');
        return;
      }

      clientHost = verifiedHost;
      request = pendingRequest;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = m('auth.oauth.request_timeout');
      } else {
        error = err instanceof Error ? err.message : m('auth.oauth.request_load_failed');
      }
    } finally {
      loading = false;
    }
  });

  function verifiedClientHost(pendingRequest: ConsentRequest) {
    try {
      const redirectUri = new URL(pendingRequest.redirectUri);
      if (redirectUri.host) {
        const redirectOrigin = new URL(pendingRequest.redirectOrigin);
        if (
          redirectUri.protocol !== redirectOrigin.protocol ||
          redirectUri.hostname !== redirectOrigin.hostname ||
          redirectUri.port !== redirectOrigin.port
        ) {
          return '';
        }
      } else if (pendingRequest.redirectOrigin !== redirectUri.protocol) {
        return '';
      }

      if (!pendingRequest.clientId) {
        return redirectUri.host;
      }
      return new URL(pendingRequest.clientId).host;
    } catch {
      return '';
    }
  }

  function isLocalCallback(raw: string) {
    try {
      const hostname = new URL(raw).hostname.toLowerCase();
      return (
        hostname === 'localhost' ||
        hostname === '127.0.0.1' ||
        hostname === '::1' ||
        hostname === '[::1]' ||
        (hostname.endsWith('.localhost') && hostname.length > '.localhost'.length)
      );
    } catch {
      return false;
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
        error = result.error || m('auth.oauth.submit_failed');
        return;
      }
      if (!result.redirectUrl) {
        error = m('auth.oauth.missing_redirect');
        return;
      }

      window.location.href = result.redirectUrl;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = m('auth.oauth.decision_timeout');
      } else {
        error = err instanceof Error ? err.message : m('auth.oauth.submit_failed');
      }
    } finally {
      submitting = null;
    }
  }
</script>

<PageTitle title={m('auth.oauth.title')} />

<AuthLayout compact>
  <div class="flex flex-col gap-5">
    <div class="text-center">
      <h1 class="text-2xl font-bold">{m('auth.oauth.heading')}</h1>
    </div>

    {#if loading}
      <div class="flex justify-center py-8">
        <span class="iconify icon-[mdi--loading] animate-spin text-3xl text-muted"></span>
      </div>
    {:else if request}
      <div class="flex flex-col gap-4">
        <div class="text-center">
          <p class="font-semibold break-all">{request.clientName || clientHost}</p>
          {#if request.clientName}
            <p class="mt-1 text-sm break-all text-muted">{clientHost}</p>
          {/if}
        </div>

        {#if request.localRedirect}
          <Hint tone="warning">
            {m('auth.oauth.local_callback_warning', { address: request.redirectOrigin })}
          </Hint>
        {/if}

        <div class="surface-box p-4">
          <div class="mb-3 text-sm font-medium">{m('auth.oauth.allow_intro')}</div>
          <ul class="flex flex-col gap-2 text-sm text-muted">
            {#if request.scopes.length === 0}
              <li class="flex gap-2">
                <span class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"></span>
                <span>{m('auth.oauth.allow_profile')}</span>
              </li>
              <li class="flex gap-2">
                <span class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"></span>
                <span>{m('auth.oauth.allow_messages')}</span>
              </li>
            {:else}
              {#if request.scopes.includes('chatto:rooms:read')}
                <li class="flex gap-2">
                  <span class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"></span>
                  <span>{m('auth.oauth.allow_rooms_read')}</span>
                </li>
              {/if}
              {#if request.scopes.includes('chatto:rooms:write')}
                <li class="flex gap-2">
                  <span class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"></span>
                  <span>{m('auth.oauth.allow_rooms_write')}</span>
                </li>
              {/if}
              {#if request.scopes.includes('chatto:messages:read') || request.scopes.includes('chatto:messages:write')}
                <li class="flex gap-2">
                  <span class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"></span>
                  <span>{m('auth.oauth.allow_messages')}</span>
                </li>
              {/if}
            {/if}
            {#if !request.localRedirect}
              <li class="flex gap-2">
                <span class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"></span>
                <span>{m('auth.oauth.allow_remember')}</span>
              </li>
            {/if}
          </ul>
        </div>

        <FormError {error} />

        <div class="flex flex-col gap-2">
          <Button
            size="lg"
            fullWidth
            loading={submitting === 'approve'}
            loadingText={m('auth.oauth.authorizing')}
            disabled={submitting !== null}
            onclick={() => submitConsent('approve')}
          >
            <span class="iconify icon-[mdi--check]"></span>
            {m('auth.oauth.title')}
          </Button>
          <Button
            variant="secondary"
            size="lg"
            fullWidth
            loading={submitting === 'deny'}
            loadingText={m('auth.oauth.denying')}
            disabled={submitting !== null}
            onclick={() => submitConsent('deny')}
          >
            <span class="iconify icon-[mdi--close]"></span>
            {m('common.cancel')}
          </Button>
        </div>
      </div>
    {:else}
      <div class="flex flex-col gap-4 text-center">
        <FormError {error} />
        <Button variant="secondary" size="lg" fullWidth onclick={() => goto(resolve('/'))}>
          {m('auth.oauth.return_home')}
        </Button>
      </div>
    {/if}
  </div>
</AuthLayout>
