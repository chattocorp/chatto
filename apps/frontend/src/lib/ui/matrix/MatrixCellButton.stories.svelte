<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MatrixCellButton from './MatrixCellButton.svelte';

  const componentDescription = `
  Presentational matrix control for one fixed-size cell. A domain adapter
  supplies the icon, semantic tone, explicit or inherited intensity, status
  markers, accessible label, and activation callback. This component does not
  define a state cycle.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/Matrix/Cell button',
    component: MatrixCellButton,
    tags: ['autodocs'],
    parameters: {
      docs: { description: { component: componentDescription } }
    }
  });
</script>

<Story
  name="Explicit and inherited tones"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Solid surfaces are explicit values. Quiet surfaces are effective inherited values. The link marker makes inheritance visible without reliance on intensity.'
      }
    }
  }}
>
  <div
    class="inline-grid grid-cols-[auto_repeat(5,2.5rem)] items-center gap-2 rounded-lg bg-background p-4 text-sm"
  >
    <span></span>
    <span class="text-center text-muted">Neutral</span>
    <span class="text-center text-muted">Action</span>
    <span class="text-center text-muted">Success</span>
    <span class="text-center text-muted">Warning</span>
    <span class="text-center text-muted">Danger</span>

    <span class="pe-3 text-muted">Explicit</span>
    <MatrixCellButton
      tone="neutral"
      explicit
      icon="icon-[uil--minus]"
      ariaLabel="Explicit neutral"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="action"
      explicit
      icon="icon-[uil--volume-up]"
      ariaLabel="Explicit action"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="success"
      explicit
      icon="icon-[uil--check]"
      ariaLabel="Explicit success"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="warning"
      explicit
      icon="icon-[uil--bell]"
      ariaLabel="Explicit warning"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="danger"
      explicit
      icon="icon-[uil--times]"
      ariaLabel="Explicit danger"
      onActivate={() => undefined}
    />

    <span class="pe-3 text-muted">Inherited</span>
    <MatrixCellButton
      tone="neutral"
      inheritedMarker
      icon="icon-[uil--minus]"
      ariaLabel="Inherited neutral"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="action"
      inheritedMarker
      icon="icon-[uil--volume-up]"
      ariaLabel="Inherited action"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="success"
      inheritedMarker
      icon="icon-[uil--check]"
      ariaLabel="Inherited success"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="warning"
      inheritedMarker
      icon="icon-[uil--bell]"
      ariaLabel="Inherited warning"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="danger"
      inheritedMarker
      icon="icon-[uil--times]"
      ariaLabel="Inherited danger"
      onActivate={() => undefined}
    />
  </div>
</Story>

<Story
  name="Bare icons"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The icon presentation keeps the 40-pixel hit area but removes the badge surface. Explicit values use full intensity and inherited values use the same icon at reduced intensity.'
      }
    }
  }}
>
  <div
    class="inline-grid grid-cols-[auto_repeat(3,2.5rem)] items-center gap-2 rounded-lg bg-background p-4 text-sm"
  >
    <span></span>
    <span class="text-center text-muted">Neutral</span>
    <span class="text-center text-muted">Warning</span>
    <span class="text-center text-muted">Action</span>

    <span class="pe-3 text-muted">Explicit</span>
    <MatrixCellButton
      variant="icon"
      tone="neutral"
      explicit
      icon="icon-[ph--bell-slash-fill]"
      ariaLabel="Explicit neutral icon"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      variant="icon"
      tone="warning"
      explicit
      icon="icon-[ph--bell-fill]"
      ariaLabel="Explicit warning icon"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      variant="icon"
      tone="action"
      explicit
      icon="icon-[ph--phone-fill]"
      ariaLabel="Explicit action icon"
      onActivate={() => undefined}
    />

    <span class="pe-3 text-muted">Inherited</span>
    <MatrixCellButton
      variant="icon"
      tone="neutral"
      icon="icon-[ph--bell-slash-fill]"
      ariaLabel="Inherited neutral icon"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      variant="icon"
      tone="warning"
      icon="icon-[ph--bell-fill]"
      ariaLabel="Inherited warning icon"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      variant="icon"
      tone="action"
      icon="icon-[ph--phone-fill]"
      ariaLabel="Inherited action icon"
      onActivate={() => undefined}
    />
  </div>
</Story>

<Story
  name="Status and constraints"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Pending cells remain focusable, locked and disabled cells use native disabled semantics, and markers communicate warnings or inheritance.'
      }
    }
  }}
>
  <div class="flex flex-wrap gap-6 rounded-lg bg-background p-4 text-center text-xs text-muted">
    <div>
      <MatrixCellButton
        tone="action"
        explicit
        icon="icon-[uil--check]"
        loading
        ariaLabel="Saving"
        onActivate={() => undefined}
      />
      <div>Saving</div>
    </div>
    <div>
      <MatrixCellButton
        tone="success"
        explicit
        icon="icon-[uil--check]"
        locked
        ariaLabel="Locked"
        onActivate={() => undefined}
      />
      <div>Locked</div>
    </div>
    <div>
      <MatrixCellButton
        tone="warning"
        explicit
        icon="icon-[uil--check]"
        warning
        ariaLabel="Warning"
        onActivate={() => undefined}
      />
      <div>Warning</div>
    </div>
    <div>
      <MatrixCellButton
        tone="action"
        icon="icon-[uil--check]"
        inheritedMarker
        ariaLabel="Inherited"
        onActivate={() => undefined}
      />
      <div>Inherited</div>
    </div>
    <div>
      <MatrixCellButton
        tone="neutral"
        explicit
        icon="icon-[uil--minus]"
        disabled
        ariaLabel="Disabled"
        onActivate={() => undefined}
      />
      <div>Disabled</div>
    </div>
    <div>
      <MatrixCellButton
        icon="icon-[uil--minus]"
        applicable={false}
        ariaLabel="Not applicable"
        onActivate={() => undefined}
      />
      <div>Not applicable</div>
    </div>
  </div>
</Story>

<Story
  name="Dark theme"
  asChild
  globals={{ theme: 'dark' }}
  parameters={{
    docs: {
      description: {
        story: 'Explicit, inherited, and constrained surfaces retain their meaning in dark mode.'
      }
    }
  }}
>
  <div class="flex gap-2 rounded-lg bg-background p-4">
    <MatrixCellButton
      tone="neutral"
      explicit
      icon="icon-[uil--minus]"
      ariaLabel="Explicit neutral"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="success"
      explicit
      icon="icon-[uil--check]"
      ariaLabel="Explicit success"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="warning"
      inheritedMarker
      icon="icon-[uil--bell]"
      ariaLabel="Inherited warning"
      onActivate={() => undefined}
    />
    <MatrixCellButton
      tone="danger"
      warning
      icon="icon-[uil--times]"
      ariaLabel="Inherited danger with warning"
      onActivate={() => undefined}
    />
  </div>
</Story>
