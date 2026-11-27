<!--
@component
Shows a profile's verified identities from the shared profile-details read,
so the popup and sidebar share one request per server session and profile.
-->
<script lang="ts">
  import { useProfileDetails } from './profileDetailsQuery.svelte';
  import VerifiedIdentitiesSection from './VerifiedIdentitiesSection.svelte';

  let { userId, plain = false }: { userId: string; plain?: boolean } = $props();

  const details = useProfileDetails(() => userId);
</script>

<VerifiedIdentitiesSection identities={details.data?.verifiedIdentities ?? []} {plain} />
