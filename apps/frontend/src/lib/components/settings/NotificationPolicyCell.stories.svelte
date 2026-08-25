<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NotificationPolicyCell from './NotificationPolicyCell.svelte';

  const componentDescription = `
  Notification-policy adapter for the shared matrix cell control. It maps the
  three delivery modes to the shared icon and colour language, displays
  effective inherited values, and owns the scope-specific state cycle. Server
  defaults are concrete starting values and do not display as inherited. The
  cell keeps its detailed state and next action in its accessible label without
  showing that text as a native browser tooltip.
  `.trim();

  const { Story } = defineMeta({
    title: 'Settings/Notification policy cell',
    component: NotificationPolicyCell,
    tags: ['autodocs'],
    parameters: {
      docs: { description: { component: componentDescription } }
    }
  });
</script>

<script lang="ts">
  import { NotificationDeliveryMode } from '$lib/api-client/notifications';
</script>

<Story
  name="Delivery modes"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Grey means no notification occurrence. Orange means an in-app notification. The phone icon adds push delivery.'
      }
    }
  }}
>
  <div class="flex flex-wrap gap-6 rounded-lg bg-background p-4 text-center text-xs text-muted">
    <div>
      <NotificationPolicyCell
        field="directMessages"
        causeLabel="Direct messages"
        scope={{ kind: 'server' }}
        scopeLabel="Example server"
        override={NotificationDeliveryMode.OFF}
        effective={NotificationDeliveryMode.OFF}
        onChange={() => undefined}
      />
      <div>Off</div>
    </div>
    <div>
      <NotificationPolicyCell
        field="directMentions"
        causeLabel="Direct mentions"
        scope={{ kind: 'server' }}
        scopeLabel="Example server"
        override={NotificationDeliveryMode.IN_APP_NOTIFICATION}
        effective={NotificationDeliveryMode.IN_APP_NOTIFICATION}
        onChange={() => undefined}
      />
      <div>Notification</div>
    </div>
    <div>
      <NotificationPolicyCell
        field="replies"
        causeLabel="Replies"
        scope={{ kind: 'server' }}
        scopeLabel="Example server"
        override={NotificationDeliveryMode.PUSH_NOTIFICATION}
        effective={NotificationDeliveryMode.PUSH_NOTIFICATION}
        onChange={() => undefined}
      />
      <div>Push notification</div>
    </div>
  </div>
</Story>

<Story
  name="Defaults and inheritance"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'An unconfigured server cell displays its product default at full intensity. Nested cells show an inherited server or room-group value with the same icon at reduced intensity.'
      }
    }
  }}
>
  <div class="flex flex-wrap gap-6 rounded-lg bg-background p-4 text-center text-xs text-muted">
    <div>
      <NotificationPolicyCell
        field="directMessages"
        causeLabel="Direct messages"
        scope={{ kind: 'server' }}
        scopeLabel="Example server"
        override={null}
        effective={NotificationDeliveryMode.PUSH_NOTIFICATION}
        onChange={() => undefined}
      />
      <div>Server default</div>
    </div>
    <div>
      <NotificationPolicyCell
        field="directMessages"
        causeLabel="Direct messages"
        scope={{ kind: 'roomGroup', id: 'community' }}
        scopeLabel="Community"
        override={null}
        effective={NotificationDeliveryMode.PUSH_NOTIFICATION}
        onChange={() => undefined}
      />
      <div>Inherited by group</div>
    </div>
    <div>
      <NotificationPolicyCell
        field="directMentions"
        causeLabel="Direct mentions"
        scope={{ kind: 'room', id: 'general' }}
        scopeLabel="general"
        override={null}
        effective={NotificationDeliveryMode.OFF}
        onChange={() => undefined}
      />
      <div>Inherited by room</div>
    </div>
  </div>
</Story>

<Story
  name="Saving and disabled"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'A pending cell keeps keyboard focus but rejects repeat activation. Disabled cells use native disabled semantics.'
      }
    }
  }}
>
  <div class="flex gap-6 rounded-lg bg-background p-4 text-center text-xs text-muted">
    <div>
      <NotificationPolicyCell
        field="replies"
        causeLabel="Replies"
        scope={{ kind: 'room', id: 'general' }}
        scopeLabel="general"
        override={NotificationDeliveryMode.PUSH_NOTIFICATION}
        effective={NotificationDeliveryMode.PUSH_NOTIFICATION}
        loading
        onChange={() => undefined}
      />
      <div>Saving</div>
    </div>
    <div>
      <NotificationPolicyCell
        field="reactions"
        causeLabel="Reactions"
        scope={{ kind: 'room', id: 'general' }}
        scopeLabel="general"
        override={NotificationDeliveryMode.IN_APP_NOTIFICATION}
        effective={NotificationDeliveryMode.IN_APP_NOTIFICATION}
        disabled
        onChange={() => undefined}
      />
      <div>Disabled</div>
    </div>
  </div>
</Story>
