<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SubjectPermissionsMatrix, { type MatrixData } from './SubjectPermissionsMatrix.svelte';

  const data: MatrixData = {
    applicablePermissions: ['message.post', 'message.react', 'room.manage', 'room.view-history'],
    scopes: [
      { id: 'server', label: 'Example server', kind: 'SERVER', parentGroupId: '' },
      { id: 'group:community', label: 'Community', kind: 'GROUP', parentGroupId: '' },
      { id: 'room:general', label: 'general', kind: 'ROOM', parentGroupId: 'community' },
      { id: 'room:support', label: 'support', kind: 'ROOM', parentGroupId: 'community' }
    ],
    cells: [
      { permission: 'message.post', scopeId: 'server', override: 'ALLOW', effective: 'ALLOW' },
      {
        permission: 'message.post',
        scopeId: 'group:community',
        override: 'NONE',
        effective: 'ALLOW'
      },
      { permission: 'message.post', scopeId: 'room:general', override: 'NONE', effective: 'ALLOW' },
      { permission: 'message.post', scopeId: 'room:support', override: 'DENY', effective: 'DENY' },
      { permission: 'message.react', scopeId: 'server', override: 'NONE', effective: 'NONE' },
      {
        permission: 'message.react',
        scopeId: 'group:community',
        override: 'ALLOW',
        effective: 'ALLOW'
      },
      {
        permission: 'message.react',
        scopeId: 'room:general',
        override: 'NONE',
        effective: 'ALLOW'
      },
      {
        permission: 'message.react',
        scopeId: 'room:support',
        override: 'NONE',
        effective: 'ALLOW'
      },
      { permission: 'room.manage', scopeId: 'server', override: 'NONE', effective: 'NONE' },
      {
        permission: 'room.manage',
        scopeId: 'group:community',
        override: 'NONE',
        effective: 'NONE'
      },
      { permission: 'room.manage', scopeId: 'room:general', override: 'ALLOW', effective: 'ALLOW' },
      { permission: 'room.manage', scopeId: 'room:support', override: 'NONE', effective: 'NONE' },
      { permission: 'room.view-history', scopeId: 'server', override: 'DENY', effective: 'DENY' },
      {
        permission: 'room.view-history',
        scopeId: 'group:community',
        override: 'NONE',
        effective: 'DENY'
      },
      {
        permission: 'room.view-history',
        scopeId: 'room:general',
        override: 'ALLOW',
        effective: 'ALLOW'
      },
      {
        permission: 'room.view-history',
        scopeId: 'room:support',
        override: 'NONE',
        effective: 'DENY'
      }
    ]
  };

  const componentDescription = `
  RBAC consumer of the shared matrix primitives. The server column comes first,
  each room-group column precedes its rooms, and the adapter keeps permission
  cycling and delegation rules outside the table primitive.
  `.trim();

  const { Story } = defineMeta({
    title: 'Admin/RBAC permission matrix',
    component: SubjectPermissionsMatrix,
    tags: ['autodocs'],
    parameters: {
      docs: { description: { component: componentDescription } }
    }
  });
</script>

<Story
  name="Scoped permissions"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'This full consumer shows scope ordering, filtering, explicit decisions, and inherited effective decisions.'
      }
    }
  }}
>
  <div class="max-w-4xl">
    <SubjectPermissionsMatrix {data} subjectKind="role" onCycle={() => undefined} />
  </div>
</Story>

<Story
  name="Narrow viewport"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'The permission identifiers stay visible while scope columns scroll horizontally.'
      }
    }
  }}
>
  <div class="w-80">
    <SubjectPermissionsMatrix {data} subjectKind="role" onCycle={() => undefined} />
  </div>
</Story>
