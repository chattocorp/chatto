<!--
@component

Permissions section of a bot: its direct grants, capped by the owner's
authority, with help about the grants that a new bot gets by default. Only
the owner and bot managers can open it.
-->
<script lang="ts">
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { Hint } from '$lib/ui';
  import { useBotDetail } from '../botDetailContext';

  const detail = useBotDetail();
</script>

<!-- Keep the matrix owner while the bot read is pending. -->
{#if detail.isPending || detail.canOperateBot}
  <Hint tone="info">{m('settings.bots.permissions.defaults')}</Hint>
  <UserPermissionsMatrix
    userId={detail.botId}
    subjectKind={m('settings.bots.singular')}
    ownerCapped
    decisionMode="binary"
  />
{:else}
  <Hint tone="danger">{m('ui.access_denied.message')}</Hint>
{/if}
