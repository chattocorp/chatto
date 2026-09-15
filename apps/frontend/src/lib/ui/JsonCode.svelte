<!-- @component
Display JSON text with the shared syntax colours. Preserve the source text and
show it without highlighting while the language loads or if highlighting fails.
-->
<script lang="ts">
  import ScrollArea from './ScrollArea.svelte';

  let { text }: { text: string } = $props();

  type Token =
    | { type: 'text'; value: string }
    | {
        type: 'element';
        properties: { className?: string[] };
        children: Token[];
      };

  async function highlight(source: string): Promise<Token[] | null> {
    try {
      const { ensureCodeLanguageLoaded, isCodeLanguageLoaded, lowlight } =
        await import('$lib/codeHighlighting');
      await ensureCodeLanguageLoaded('json');
      if (!isCodeLanguageLoaded('json')) return null;
      return lowlight.highlight('json', source).children as Token[];
    } catch {
      return null;
    }
  }

  const tokens = $derived(highlight(text));
</script>

{#snippet renderTokens(nodes: Token[])}
  {#each nodes as node (node)}
    {#if node.type === 'text'}{node.value}{:else}<span class={node.properties.className}
        >{@render renderTokens(node.children)}</span
      >{/if}
  {/each}
{/snippet}

<ScrollArea scrollX fill={false} aria-label="JSON">
  <pre
    dir="ltr"
    class="composer-code-palette w-max min-w-full rounded-md bg-surface-emphasized p-4 font-mono text-xs leading-relaxed text-(--composer-code-text) [&_.hljs-attr]:text-(--composer-code-attribute) [&_.hljs-literal]:text-(--composer-code-literal) [&_.hljs-number]:text-(--composer-code-literal) [&_.hljs-string]:text-(--composer-code-string)"><code
      >{#await tokens}{text}{:then nodes}{#if nodes}{@render renderTokens(
            nodes
          )}{:else}{text}{/if}{/await}</code
    ></pre>
</ScrollArea>
