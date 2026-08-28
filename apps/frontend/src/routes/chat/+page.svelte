<script lang="ts">
  import ChatLandingRedirect from './ChatLandingRedirect.svelte';

  let { data } = $props();
</script>

<ChatLandingRedirect user={data.user} welcome={data.welcome} />
