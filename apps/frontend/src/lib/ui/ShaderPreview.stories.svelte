<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ShaderPreview from './ShaderPreview.svelte';
  const { Story } = defineMeta({ title: 'UI/ShaderPreview', component: ShaderPreview });
</script>

<script lang="ts">
  import MarkdownHtml from './MarkdownHtml.svelte';
  import { renderMarkdown } from '$lib/markdown';
  import functionPresentation from './shaderDemos/functionPresentation.glsl?raw';
  import eventHorizon from './shaderDemos/eventHorizon.glsl?raw';

  const plasma = `void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  vec2 uv = fragCoord / iResolution.xy;
  vec3 color = 0.5 + 0.5 * cos(iTime + uv.xyx + vec3(0.0, 2.0, 4.0));
  fragColor = vec4(color, 1.0);
}`;
  // Shows a glow at the pointer: orange while pressed, blue after release.
  const mouse = `void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  vec2 uv = fragCoord / iResolution.xy;
  float glow = 0.02 / max(distance(fragCoord, iMouse.xy) / iResolution.y, 0.005);
  vec3 base = 0.15 + 0.1 * cos(iTime + uv.xyx * 3.0 + vec3(0.0, 2.0, 4.0));
  vec3 color = base + glow * (iMouse.z > 0.0 ? vec3(1.0, 0.6, 0.2) : vec3(0.3, 0.6, 1.0));
  fragColor = vec4(color, 1.0);
}`;
  const fence = '```';
  const html = renderMarkdown(
    `Code example:\n\n${fence}glsl\n${plasma}\n${fence}\n\nVisual message:\n\n${fence}glsl rendered\n${plasma}\n${fence}\n\nInvalid shader:\n\n${fence}glsl rendered\nvoid mainImage(out vec4 c, in vec2 p) { c = nope; }\n${fence}`,
    { shaders: true }
  );
</script>

<Story name="Plasma" args={{ source: plasma }} />

<Story name="Mouse" args={{ source: mouse }} />

<Story name="Function presentation" args={{ source: functionPresentation }} />

<Story name="Event horizon" args={{ source: eventHorizon }} />

<Story name="In Markdown" asChild>
  {#await html then markup}
    <div class="prose max-w-2xl">
      <MarkdownHtml html={markup} />
    </div>
  {/await}
</Story>
