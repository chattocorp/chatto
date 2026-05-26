<script lang="ts">
  import ScrollFader from './ScrollFader.svelte';

  let refreshKey = $state(0);
  let scrollEl = $state<HTMLDivElement>();

  export function refresh() {
    refreshKey++;
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

<ScrollFader top bottom bind:scrollEl {refreshKey} data-testid="scroll">
  <div data-testid="content">Message</div>
</ScrollFader>
