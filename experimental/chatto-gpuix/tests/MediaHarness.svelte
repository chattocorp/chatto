<script lang="ts">
  import Theme from '../src/ui/Theme.svelte';
  import NativeImage from '../src/NativeImage.svelte';
  let active = $state(false);
  let metadata = $state({ revision: 0 });
  async function resolveSource() {
    void metadata.revision;
    return src;
  }
  /** Simulate a store refresh that replaces metadata for the same image. */
  export function updateMetadata() {
    metadata = { revision: metadata.revision + 1 };
  }
  let src = $state('https://images.example.test/preview.png');
  /** Drive viewport changes through the same reactive prop used by the timeline. */
  export function setActive(value: boolean) {
    active = value;
  }
  /** Replace a signed URL or image while retaining the mounted row. */
  export function setSource(value: string) {
    src = value;
  }
</script>

<Theme>
  <div style="width: 440px; height: 300px;">
    <NativeImage
      {src}
      {active}
      serverUrl="https://chat.example.test"
      alt="Fixture"
      {resolveSource}
    />
  </div>
</Theme>
