<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CollapsibleGroupStack from './CollapsibleGroupStack.svelte';

  type NavigationItem = {
    id: string;
    label: string;
    kind: 'room' | 'dm';
    initial?: string;
  };

  type MemberItem = {
    id: string;
    name: string;
    initial: string;
    status: 'online' | 'offline';
  };

  type FileItem = {
    id: string;
    filename: string;
    time: string;
    icon: string;
  };

  const { Story } = defineMeta({
    title: 'UI/CollapsibleGroupStack',
    component: CollapsibleGroupStack,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  const groups = [
    {
      id: 'rooms',
      label: 'Testing',
      persistKey: 'storybook:collapsible-group-stack:rooms',
      items: [
        { id: 'general', label: 'general', kind: 'room' as const },
        { id: 'announcements', label: 'announcements', kind: 'room' as const }
      ]
    },
    {
      id: 'direct-messages',
      label: 'Direct Messages',
      persistKey: 'storybook:collapsible-group-stack:dms',
      items: [
        { id: 'nick', label: 'nick', kind: 'dm' as const, initial: 'N' },
        { id: 'self', label: 'You', kind: 'dm' as const, initial: 'H' }
      ]
    }
  ];

  const memberGroups = [
    {
      id: 'online',
      label: 'Online — 2',
      persistKey: 'storybook:collapsible-group-stack:members:online',
      items: [
        { id: 'teal', name: 'Teal', initial: 'T', status: 'online' as const },
        { id: 'river', name: 'River', initial: 'R', status: 'online' as const }
      ]
    },
    {
      id: 'offline',
      label: 'Offline — 1',
      persistKey: 'storybook:collapsible-group-stack:members:offline',
      defaultCollapsed: true,
      items: [{ id: 'nick', name: 'Nick', initial: 'N', status: 'offline' as const }]
    }
  ];

  const fileGroups = [
    {
      id: 'today',
      label: 'Today',
      persistKey: 'storybook:collapsible-group-stack:files:today',
      items: [
        {
          id: 'roadmap',
          filename: 'streaming-roadmap.pdf',
          time: 'Today at 14:38',
          icon: 'icon-[uil--file-alt]'
        },
        {
          id: 'capture',
          filename: 'game-capture.png',
          time: 'Today at 12:04',
          icon: 'icon-[uil--image]'
        }
      ]
    },
    {
      id: 'yesterday',
      label: 'Yesterday',
      persistKey: 'storybook:collapsible-group-stack:files:yesterday',
      items: [
        {
          id: 'notes',
          filename: 'broadcast-notes.txt',
          time: 'Yesterday at 18:22',
          icon: 'icon-[uil--file-alt]'
        }
      ]
    }
  ];
</script>

{#snippet navigationItem(item: NavigationItem)}
  <button type="button" class="sidebar-item w-full text-start">
    {#if item.kind === 'room'}
      <span class="sidebar-icon text-muted">#</span>
    {:else}
      <span
        class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-xs font-medium text-muted"
        >{item.initial}</span
      >
    {/if}
    <span>{item.label}</span>
  </button>
{/snippet}

{#snippet memberItem(item: MemberItem)}
  <button type="button" class="sidebar-item w-full text-start">
    <span class="relative shrink-0">
      <span
        class="flex h-6 w-6 items-center justify-center rounded-full bg-surface-emphasized text-xs font-medium text-muted"
        >{item.initial}</span
      >
      <span
        class={[
          'absolute end-0 bottom-0 h-2 w-2 rounded-full border border-background',
          item.status === 'online' ? 'bg-success' : 'bg-muted'
        ]}
      ></span>
    </span>
    <span>{item.name}</span>
  </button>
{/snippet}

{#snippet fileItem(item: FileItem)}
  <button type="button" class="sidebar-item min-h-14 w-full gap-3 text-start">
    <span
      class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-border bg-surface text-muted"
    >
      <span class={['iconify text-xl', item.icon]} aria-hidden="true"></span>
    </span>
    <span class="min-w-0 flex-1">
      <span class="block truncate text-sm">{item.filename}</span>
      <span class="block truncate text-xs text-muted">{item.time}</span>
    </span>
  </button>
{/snippet}

<Story name="Sidebar sections" asChild>
  <div class="w-64 rounded-lg bg-background p-2">
    <CollapsibleGroupStack {groups} item={navigationItem} />
  </div>
</Story>

<Story name="Single section" asChild>
  <div class="w-64 rounded-lg bg-background p-2">
    <CollapsibleGroupStack groups={groups.slice(0, 1)} item={navigationItem} />
  </div>
</Story>

<Story name="Member presence" asChild>
  <div class="w-64 rounded-lg bg-background p-2">
    <CollapsibleGroupStack groups={memberGroups} item={memberItem} />
  </div>
</Story>

<Story name="Attachment dates" asChild>
  <div class="w-72 rounded-lg bg-background p-2">
    <CollapsibleGroupStack groups={fileGroups} item={fileItem} />
  </div>
</Story>
