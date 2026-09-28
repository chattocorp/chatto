<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { csrfFetch } from '$lib/auth/csrf';
  import { LOOPBACK_OAUTH_CLIENT_ID } from '$lib/auth/loopbackClient';
  import AuthLayout from '$lib/components/AuthLayout.svelte';
  import { m } from '$lib/i18n/messages';
  import Hint from '$lib/ui/Hint.svelte';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
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
  let clientIdentity = $state('');
  // The loopback client shows a name that does not endorse it, and its
  // callback origin instead of its ID.
  const clientDisplayName = $derived(
    request?.clientId === LOOPBACK_OAUTH_CLIENT_ID
      ? m('auth.oauth.loopback_client_name')
      : (request?.clientName ?? '')
  );
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
      const verifiedIdentity = verifiedClientIdentity(pendingRequest);
      if (!verifiedIdentity) {
        error = m('auth.oauth.unverifiable');
        return;
      }

      clientIdentity = verifiedIdentity;
      request = pendingRequest;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = m('auth.oauth.request_timeout');
      } else {
        error = errorMessage(err, m('auth.oauth.request_load_failed'));
      }
    } finally {
      loading = false;
    }
  });

  /** Check callback consistency and display the identity already validated by the server. */
  function verifiedClientIdentity(pendingRequest: ConsentRequest) {
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
      if (pendingRequest.clientId === LOOPBACK_OAUTH_CLIENT_ID) {
        return pendingRequest.redirectOrigin;
      }
      if (typeof pendingRequest.clientId !== 'string') return '';
      // CIMD IDs are URLs; built-in native IDs can be opaque strings. Keep
      // the exact server-validated ID visible instead of using its website
      // or display name as an identity fallback.
      try {
        return new URL(pendingRequest.clientId).host || pendingRequest.clientId;
      } catch {
        return pendingRequest.clientId;
      }
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
        error = errorMessage(err, m('auth.oauth.submit_failed'));
      }
    } finally {
      submitting = null;
    }
  }
</script>

<PageTitle title={m('auth.oauth.title')} />

<AuthLayout compact title={m('auth.oauth.heading')}>
  <div class="flex flex-col gap-5">
    {#if loading}
      <LoadingFog class="h-48 w-full" />
    {:else if request}
      <div class="flex flex-col gap-4">
        <div class="text-center">
          <p class="font-semibold break-all">{clientDisplayName || clientIdentity}</p>
          {#if clientDisplayName}
            <p class="mt-1 text-sm break-all text-muted">{clientIdentity}</p>
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
                <span
                  aria-hidden="true"
                  class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"
                ></span>
                <span>{m('auth.oauth.allow_profile')}</span>
              </li>
              <li class="flex gap-2">
                <span
                  aria-hidden="true"
                  class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"
                ></span>
                <span>{m('auth.oauth.allow_messages')}</span>
              </li>
            {:else}
              {#if request.scopes.includes('chatto:rooms:read')}
                <li class="flex gap-2">
                  <span
                    aria-hidden="true"
                    class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"
                  ></span>
                  <span>{m('auth.oauth.allow_rooms_read')}</span>
                </li>
              {/if}
              {#if request.scopes.includes('chatto:rooms:write')}
                <li class="flex gap-2">
                  <span
                    aria-hidden="true"
                    class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"
                  ></span>
                  <span>{m('auth.oauth.allow_rooms_write')}</span>
                </li>
              {/if}
              {#if request.scopes.includes('chatto:messages:read') || request.scopes.includes('chatto:messages:write')}
                <li class="flex gap-2">
                  <span
                    aria-hidden="true"
                    class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"
                  ></span>
                  <span>{m('auth.oauth.allow_messages')}</span>
                </li>
              {/if}
            {/if}
            {#if !request.localRedirect}
              <li class="flex gap-2">
                <span
                  aria-hidden="true"
                  class="iconify mt-0.5 icon-[mdi--check] shrink-0 text-action"
                ></span>
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
            <span aria-hidden="true" class="iconify icon-[mdi--check]"></span>
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
            <span aria-hidden="true" class="iconify icon-[mdi--close]"></span>
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
