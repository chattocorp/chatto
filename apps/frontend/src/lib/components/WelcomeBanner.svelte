<!--
@component

Shows a welcome banner to newly verified users. The banner auto-dismisses
after 5 seconds and can be manually dismissed. When shown, removes the
`welcome` query parameter from the URL.

Only renders when the `welcome=true` query parameter is present.
-->
<script lang="ts">
  import { page } from '$app/state';
  import * as m from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';

  let showWelcome = $state(
    page.url.searchParams.get('welcome') === 'true' || page.state.welcome === true
  );

  function mountWelcomeBanner() {
    // Remove the query param from URL without navigation.
    const url = new URL(window.location.href);
    url.searchParams.delete('welcome');
    window.history.replaceState({}, '', url.toString());

    const timer = setTimeout(() => {
      showWelcome = false;
    }, 5000);
    return () => clearTimeout(timer);
  }
</script>

{#if showWelcome}
  <div class="mb-2" {@attach mountWelcomeBanner}>
    <Hint tone="success">
      <div class="flex items-start justify-between gap-3">
        <span>{m['welcome.verified']()}</span>
        <button
          type="button"
          class="-m-1 icon-action"
          onclick={() => (showWelcome = false)}
          title={m['common.dismiss']()}
        >
          <span class="iconify uil--times"></span>
        </button>
      </div>
    </Hint>
  </div>
{/if}
