<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MatrixCell from './MatrixCell.svelte';

  const componentDescription = `
  RBAC adapter for the shared matrix cell control. It owns permission cycling,
  inherited effective values, binary mode, delegation ceilings, and lock rules.
  `.trim();

  const { Story } = defineMeta({
    title: 'Admin/RBAC matrix cell',
    component: MatrixCell,
    tags: ['autodocs'],
    parameters: {
      docs: { description: { component: componentDescription } }
    }
  });
</script>

<Story
  name="Permission states"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The adapter distinguishes explicit allow and deny decisions from inherited effective decisions.'
      }
    }
  }}
>
  <div class="flex flex-wrap gap-6 rounded-lg bg-background p-4 text-center text-xs text-muted">
    <div>
      <MatrixCell override="neutral" ariaLabel="No decision" onCycle={() => undefined} />
      <div>No decision</div>
    </div>
    <div>
      <MatrixCell override="allow" ariaLabel="Explicit allow" onCycle={() => undefined} />
      <div>Allow</div>
    </div>
    <div>
      <MatrixCell override="deny" ariaLabel="Explicit deny" onCycle={() => undefined} />
      <div>Deny</div>
    </div>
    <div>
      <MatrixCell
        override="neutral"
        inherited="allow"
        ariaLabel="Inherited allow"
        onCycle={() => undefined}
      />
      <div>Inherited allow</div>
    </div>
    <div>
      <MatrixCell
        override="neutral"
        inherited="deny"
        ariaLabel="Inherited deny"
        onCycle={() => undefined}
      />
      <div>Inherited deny</div>
    </div>
  </div>
</Story>

<Story
  name="Ceilings and saving"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'A lock means the control is inert. A warning means that the configured decision is constrained but can still be removed.'
      }
    }
  }}
>
  <div class="flex flex-wrap gap-6 rounded-lg bg-background p-4 text-center text-xs text-muted">
    <div>
      <MatrixCell
        override="neutral"
        updating
        ariaLabel="Saving permission"
        onCycle={() => undefined}
      />
      <div>Saving</div>
    </div>
    <div>
      <MatrixCell
        override="neutral"
        decisionMode="binary"
        allowBlocked
        ariaLabel="Cannot grant"
        onCycle={() => undefined}
      />
      <div>Cannot grant</div>
    </div>
    <div>
      <MatrixCell
        override="allow"
        ceilingBlocked
        ariaLabel="Dormant allow"
        onCycle={() => undefined}
      />
      <div>Dormant allow</div>
    </div>
    <div>
      <MatrixCell
        override="neutral"
        inherited="allow"
        decisionMode="binary"
        locked
        ariaLabel="Inherited binary grant"
        onCycle={() => undefined}
      />
      <div>Locked inheritance</div>
    </div>
    <div>
      <MatrixCell
        override="neutral"
        applicable={false}
        ariaLabel="Not applicable"
        onCycle={() => undefined}
      />
      <div>Not applicable</div>
    </div>
  </div>
</Story>
