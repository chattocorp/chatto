<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ScrollFader from './ScrollFader.svelte';

  const componentDescription = `
A scrollable region with optional **top** and **bottom** fade overlays that
auto-hide when the scroll is at the matching edge. Useful for visually
linking scrollable content to adjacent chrome (headers, footers,
composers) so the content "fades out" before bumping into a static
boundary.

### How it composes

\`ScrollFader\` composes the reusable \`ScrollArea\` positioning wrapper and
inner scroll container. Children render **inside** the scroll container, so the
recommended pattern is to nest your own padding/layout wrapper:

\`\`\`svelte
<ScrollFader top bottom>
  <div class="flex flex-col gap-2 p-3">
    <!-- content here -->
  </div>
</ScrollFader>
\`\`\`

Reserve \`scrollClass\` for properties that target the scroll container
itself (e.g. \`overscroll-y-contain\`, \`scrollbar-hide\`). Padding,
gap, alignment, and \`[&>*]\` selectors belong on the nested wrapper.

Set \`fill={false}\` for a scrollable region that should retain its intrinsic
height up to a max-height, such as a linked-message preview.

### Plumbing the scroll element out

When something outside \`ScrollFader\` needs the scroll element —
\`virtua\`'s \`scrollRef\`, an imperative \`scrollTop\` write, or a
shared scroll-state registration — bind it via \`bind:scrollEl\`:

\`\`\`svelte
<ScrollFader top bottom bind:scrollEl={scrollContainer}>
  <!-- children -->
  <Virtualizer scrollRef={scrollContainer} ... />
</ScrollFader>
\`\`\`

### Forwarded props

Any extra props (e.g. \`data-testid\`, \`onwheel\`, \`ontouchmove\`)
are forwarded onto the inner scroll container so callers don't lose
access to native attributes / events.

### Edge detection

The fade visibility is driven by a Svelte attachment that listens to
the scroll container's \`scroll\` event plus a \`ResizeObserver\`. The
fades use \`transition-opacity\` so the show/hide is animated.
`.trim();

  const { Story } = defineMeta({
    title: 'UI/ScrollFader',
    component: ScrollFader,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<Story
  name="Top and bottom"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The default pairing: both fades on. The top fade hides while scrolled to the top; the bottom fade hides while scrolled to the bottom.'
      }
    }
  }}
>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story
  name="Bottom only"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Useful when the scroll region sits flush against a header (no top boundary worth fading into) but has a composer/footer below.'
      }
    }
  }}
>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story
  name="Sticky table"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'A horizontally scrollable table can use the same edge fades, with the top fade starting below its sticky header.'
      }
    }
  }}
>
  <div class="flex h-80 w-96 flex-col border border-border bg-background">
    <ScrollFader top bottom scrollX topFadeOffset="top-12">
      <table class="w-[40rem] text-sm">
        <thead class="sticky top-0 bg-surface">
          <tr><th class="px-3 py-2 text-left">Permission</th><th>Owner</th><th>Admin</th></tr>
        </thead>
        <tbody>
          {#each Array(24) as _, i (i)}
            <tr class="border-t border-border"><td class="px-3 py-2">message.permission-{i + 1}</td><td>✓</td><td>✓</td></tr>
          {/each}
        </tbody>
      </table>
    </ScrollFader>
  </div>
</Story>

<Story
  name="Top only"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Mirror of the previous variant: a chrome element above, free space below.'
      }
    }
  }}
>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story
  name="Short content (no fades)"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          "When content fits without scrolling, both edges count as 'at edge' and both fades stay hidden — no visual noise."
      }
    }
  }}
>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(3) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>

<Story
  name="Sandwiched between header and footer"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The canonical usage: ScrollFader sits in a flex column between two static rows so the fades visually bridge the boundaries.'
      }
    }
  }}
>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <div class="border-b border-border bg-surface px-3 py-2 text-sm font-semibold">Header</div>
    <ScrollFader top bottom>
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
    <div class="border-t border-border bg-surface px-3 py-2 text-sm font-semibold">Footer</div>
  </div>
</Story>

<Story
  name="Taller fade overlay"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          '`fadeHeight` accepts any Tailwind height utility. Larger values produce a softer, more gradual transition.'
      }
    }
  }}
>
  <div class="flex h-80 w-64 flex-col border border-border bg-background">
    <ScrollFader top bottom fadeHeight="h-16">
      <div class="flex flex-col gap-2 p-3">
        {#each Array(40) as _, i (i)}
          <div class="rounded-md bg-surface px-3 py-2 text-sm">Item {i + 1}</div>
        {/each}
      </div>
    </ScrollFader>
  </div>
</Story>
