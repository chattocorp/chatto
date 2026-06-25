<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';

  let { data } = $props();

  // Keep root as the app-shell decision point. `/chat` can hand control here
  // without deciding whether this frontend should show local auth or remote-server UI.
  // Authenticated redirects are handled by +page.ts so query params are preserved.
  // svelte-ignore state_referenced_locally
  if (!data.user) {
    goto(resolve('/login'), { replaceState: true });
  }
</script>

<!-- Redirect in progress -->
