<script lang="ts">
  import ScrollFader from './ScrollFader.svelte';

  let scrollEl = $state<HTMLDivElement>();
  let scrollFader = $state<{ refresh: () => void }>();
  let showExtraChild = $state(false);

  export function refresh() {
    scrollFader?.refresh();
  }

  export function toggleExtraChild() {
    showExtraChild = !showExtraChild;
  }

  export function setScrollMetrics(metrics: {
    scrollTop: number;
    scrollHeight: number;
    clientHeight: number;
  }) {
    if (!scrollEl) throw new Error('scroll container not rendered');

    let scrollTop = metrics.scrollTop;

    Object.defineProperties(scrollEl, {
      scrollTop: {
        configurable: true,
        get: () => scrollTop,
        set: (value) => {
          scrollTop = value;
        }
      },
      scrollHeight: {
        configurable: true,
        get: () => metrics.scrollHeight
      },
      clientHeight: {
        configurable: true,
        get: () => metrics.clientHeight
      }
    });
  }
</script>

<ScrollFader top bottom bind:this={scrollFader} bind:scrollEl data-testid="scroll">
  <div data-testid="content">Message</div>
  {#if showExtraChild}
    <div data-testid="extra-content">Extra message</div>
  {/if}
</ScrollFader>
