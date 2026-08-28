<script lang="ts">
  import { resolve } from '$app/paths';
  import type { AdminMember } from '$lib/api-client/adminUsers';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Panel } from '$lib/ui';
  import { Button } from '$lib/ui/form';

  let { member }: { member: AdminMember } = $props();

  const serverScope = useServerScope();
  const href = $derived(
    resolve('/chat/[serverId]/manage/server/members/[userId]/delete', {
      serverId: serverIdToSegment(serverScope.serverId),
      userId: member.id
    })
  );
</script>

<!-- @component Danger-zone entry point on the admin member detail page that links to the full-page account deletion confirmation. -->
<Panel title={m('admin.members.danger_zone')} icon="iconify icon-[uil--exclamation-triangle]">
  <div class="max-w-md">
    <p class="mb-4 text-sm text-muted">{m('admin.members.delete_account_entry_description')}</p>
    <Button variant="danger" {href}>{m('admin.members.open_delete_account')}</Button>
  </div>
</Panel>
