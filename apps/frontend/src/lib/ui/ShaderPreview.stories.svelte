<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ShaderPreview from './ShaderPreview.svelte';
  const { Story } = defineMeta({ title: 'UI/ShaderPreview', component: ShaderPreview });
</script>

<script lang="ts">
  import MarkdownHtml from './MarkdownHtml.svelte';
  import { renderMarkdown } from '$lib/markdown';

  const plasma = `void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  vec2 uv = fragCoord / iResolution.xy;
  vec3 color = 0.5 + 0.5 * cos(iTime + uv.xyx + vec3(0.0, 2.0, 4.0));
  fragColor = vec4(color, 1.0);
}`;
  const fence = '```';
  const html = renderMarkdown(
    `Code example:\n\n${fence}glsl\n${plasma}\n${fence}\n\nVisual message:\n\n${fence}glsl rendered\n${plasma}\n${fence}\n\nInvalid shader:\n\n${fence}glsl rendered\nvoid mainImage(out vec4 c, in vec2 p) { c = nope; }\n${fence}`,
    { shaders: true }
  );
</script>

<Story name="Plasma" args={{ source: plasma }} />

<Story name="In Markdown" asChild>
  {#await html then markup}
    <div class="prose max-w-2xl">
      <MarkdownHtml html={markup} />
    </div>
  {/await}
</Story>
