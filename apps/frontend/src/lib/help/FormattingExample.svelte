<!-- @component A local formatting sandbox with no server or account dependency. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { renderInlineMarkdown } from '$lib/markdown';
  import { MarkdownHtml, SegmentedControl } from '$lib/ui';
  import MarkdownEditorField from '$lib/components/composer/MarkdownEditorField.svelte';
  import ExampleMessage from './ExampleMessage.svelte';

  let {
    initialValue,
    showReference = true
  }: {
    /** Optional worked example supplied by the surrounding guide. */
    initialValue?: string;
    /** Hide the basic reference when the guide already explains the syntax. */
    showReference?: boolean;
  } = $props();

  let draft = $state<string | null>(null);
  const value = $derived(
    draft ?? initialValue ?? `${m('help.sample')}\n\n${m('help.design.question')}`
  );
  let editorKind = $state<'markdown' | 'visual'>('markdown');
  const options = $derived([
    { value: 'markdown' as const, label: m('settings.preferences.editor.markdown.label') },
    { value: 'visual' as const, label: m('settings.preferences.editor.visual.label') }
  ]);
  const examples = $derived([
    { key: 'bold', source: `**${m('composer.format.bold')}**` },
    { key: 'italic', source: `*${m('composer.format.italic')}*` },
    { key: 'inline_code', source: '`hello()`' }
  ]);
</script>

<section class="flex flex-col gap-5" aria-label={m('help.example')}>
  <div class="flex flex-wrap items-center justify-between gap-4">
    <h2 class="text-2xl font-semibold tracking-tight text-text-top">{m('help.example')}</h2>
    <SegmentedControl
      label={m('settings.preferences.editor.title')}
      {options}
      value={editorKind}
      onchange={(kind) => {
        editorKind = kind;
      }}
    />
  </div>
  <div class="grid min-w-0 overflow-hidden rounded-xl border border-border lg:grid-cols-2">
    <div class="flex min-w-0 flex-col gap-5 bg-surface p-5 sm:p-6">
      <MarkdownEditorField
        bind:value={
          () => value,
          (next) => {
            draft = next;
          }
        }
        {editorKind}
        label={m('help.input')}
        maxlength={2000}
      />
      <p class="text-sm leading-relaxed text-muted">{m('help.example_hint')}</p>
    </div>
    <section
      aria-label={m('help.preview')}
      class="flex min-w-0 flex-col gap-6 border-t border-border bg-background p-5 sm:p-6 lg:border-s lg:border-t-0"
    >
      <h3 class="font-semibold text-muted">{m('help.preview')}</h3>
      <ExampleMessage body={value} />
    </section>
  </div>
</section>

{#if showReference}
  <section class="flex flex-col gap-5" aria-label={m('help.design.reference')}>
    <div class="flex flex-col gap-2">
      <h2 class="text-2xl font-semibold tracking-tight text-text-top">
        {m('help.design.reference')}
      </h2>
      <p class="text-muted">{m('help.design.reference_hint')}</p>
    </div>
    <div class="grid gap-3 sm:grid-cols-3">
      {#each examples as example (example.key)}
        <button
          type="button"
          onclick={() => {
            draft = example.source;
          }}
          class="flex min-w-0 cursor-pointer flex-col items-stretch gap-4 rounded-lg border border-border p-5 text-start hover:bg-surface focus-visible:outline-2 focus-visible:outline-action"
        >
          <span class="font-semibold text-text-top">{m(`composer.format.${example.key}`)}</span>
          <code dir="ltr" class="break-words text-action">{example.source}</code>
          <div class="border-t border-border pt-4">
            <MarkdownHtml html={renderInlineMarkdown(example.source)} />
          </div>
        </button>
      {/each}
    </div>
  </section>
{/if}
