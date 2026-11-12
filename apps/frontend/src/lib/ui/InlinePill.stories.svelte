<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, waitFor } from 'storybook/test';
  import { resolve } from '$app/paths';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import EntityLinkPill from '$lib/components/messages/EntityLinkPill.svelte';
  import MessageLinkPill from '$lib/components/messages/MessageLinkPill.svelte';
  import MessageContent from '$lib/components/MessageContent.svelte';

  const { Story } = defineMeta({
    title: 'UI/Inline pills',
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'The inline-pill utility family gives native links and buttons the same shell, label, icon, and optional metadata segment. These examples use real room links, message links, and generated timestamps. Use the theme and direction controls to compare variants.'
        }
      }
    }
  });

  const room = {
    id: 'R123456789abcde',
    name: 'chatto-development',
    type: RoomKind.CHANNEL,
    isUniversal: false,
    viewerIsMember: true,
    viewerCanJoinRoom: false,
    viewerCanManageRoom: false,
    members: []
  };
  const user = {
    id: 'alice',
    login: 'alice',
    displayName: 'Alice',
    deleted: false,
    presenceStatus: PresenceStatus.OFFLINE,
    avatarUrl: null
  };

  /** Compare real consumers so their markup cannot silently drift apart. */
  async function comparePills({ canvasElement }: { canvasElement: HTMLElement }) {
    await waitFor(() => expect(canvasElement.querySelector('.message-timestamp')).not.toBeNull());
    const roomLink = canvasElement.querySelector<HTMLElement>('.entity-link')!;
    const timestamp = canvasElement.querySelector<HTMLElement>('.message-timestamp')!;
    for (const selector of ['', ' .inline-pill-main', ' .inline-pill-icon']) {
      const left = selector ? roomLink.querySelector(selector)! : roomLink;
      const right = selector ? timestamp.querySelector(selector)! : timestamp;
      for (const property of [
        'fontSize',
        'fontWeight',
        'lineHeight',
        'borderRadius',
        'paddingTop',
        'paddingBottom',
        'paddingInlineStart',
        'paddingInlineEnd',
        'gap'
      ] as const) {
        await expect(getComputedStyle(right)[property]).toBe(getComputedStyle(left)[property]);
      }
      // Mask icons use a foreground fill; text icons have a transparent background.
      if (selector !== ' .inline-pill-icon') {
        await expect(getComputedStyle(right).backgroundColor).toBe(
          getComputedStyle(left).backgroundColor
        );
      }
    }
    await expect(timestamp.getBoundingClientRect().height).toBe(
      roomLink.getBoundingClientRect().height
    );
  }
</script>

{#snippet comparison()}
  <p>
    Channel: <EntityLinkPill
      target={{ kind: 'room', serverId: 'demo', id: room.id }}
      path={resolve('/chat/[serverId]/[roomId]', { serverId: '-', roomId: room.id })}
      href="https://chat.example.test/chat/-/R123456789abcde"
      name={room.name}
      {room}
    />
  </p>
  <MessageContent
    body="Next: <t:1794513600:F>"
    timestampSettings={{ effectiveTimezone: 'UTC', effectiveHour12: false }}
    timestampLocale="en-US"
  />
{/snippet}

<Story name="Room link and timestamp" asChild play={comparePills}>
  <div class="prose max-w-xl bg-background p-4">{@render comparison()}</div>
</Story>

<Story name="Narrow text" asChild play={comparePills}>
  <div class="prose w-72 max-w-full bg-background p-4">{@render comparison()}</div>
</Story>

<Story name="Large text" asChild play={comparePills}>
  <div class="text-2xl">
    <div class="prose max-w-xl bg-background p-4">{@render comparison()}</div>
  </div>
</Story>

<Story name="Images and metadata" asChild>
  <div class="prose w-72 max-w-full space-y-4 bg-background p-4">
    <p>
      From <MessageLinkPill
        href="https://chat.example.test/chat/-/R123456789abcde/m/E123456789abcde"
        name={user.displayName}
        context="chatto-development"
        {user}
      />
    </p>
    <p>
      On <EntityLinkPill
        target={{ kind: 'server', serverId: 'demo' }}
        path={resolve('/chat/[serverId]', { serverId: '-' })}
        href="https://chat.example.test"
        name="Community server"
      />
    </p>
    <p>
      In <EntityLinkPill
        target={{ kind: 'room', serverId: 'demo', id: room.id }}
        path={resolve('/chat/[serverId]/[roomId]', { serverId: '-', roomId: room.id })}
        href="https://chat.example.test/chat/-/R123456789abcde"
        name="a-very-long-development-channel-name"
        context="Community server"
        {room}
      />
    </p>
  </div>
</Story>
