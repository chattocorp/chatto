<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import type { Role } from '$lib/components/rbac';
  import RoleMetadataPanel from './RoleMetadataPanel.svelte';

  const everyone: Role = {
    name: 'everyone',
    displayName: 'Everyone',
    description: 'All authenticated users',
    permissions: [],
    permissionDenials: [],
    isSystem: true,
    position: 0,
    pingable: false
  };

  const { Story } = defineMeta({
    title: 'Admin/Role details',
    component: RoleMetadataPanel,
    args: {
      role: everyone,
      saving: false,
      savingPingable: false,
      onSaveMetadata: () => {},
      onSavePingable: async () => true,
      onDelete: () => {}
    }
  });
</script>

<Story name="Everyone" />
<Story
  name="System role"
  args={{
    role: {
      ...everyone,
      name: 'admin',
      displayName: 'Administrators',
      description: 'Server administrators'
    }
  }}
/>
<Story
  name="Custom role"
  args={{
    role: {
      ...everyone,
      name: 'community-moderators',
      displayName: 'Community moderators',
      description: 'Keep conversations welcoming and help members find their way.',
      isSystem: false,
      pingable: true
    }
  }}
/>
