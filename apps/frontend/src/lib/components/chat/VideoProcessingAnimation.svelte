<!--
	Animated placeholder for videos while the server prepares playback variants.
	The canvas is decorative; the live status text remains accessible.
-->
<script module lang="ts">
  const TEXTURE_WIDTH = 256;
  const TEXTURE_HEIGHT = 160;
  const SOURCE_VIEW_WIDTH = 208;
  const MAX_REFINEMENT_LEVEL = 4;
  const ACTIVE_FRAME_INTERVAL = 250;
  const RESOLVED_FRAME_INTERVAL = 1000;
  const AMBIENT_REFINEMENT_SECONDS = 14;

  let sharedCloudTexture: HTMLCanvasElement | undefined;

  function mix(from: number, to: number, amount: number) {
    return from + (to - from) * amount;
  }

  function smoothstep(edge0: number, edge1: number, value: number) {
    const position = Math.min(1, Math.max(0, (value - edge0) / (edge1 - edge0)));
    return position * position * (3 - 2 * position);
  }

  function hash(x: number, y: number) {
    let value = Math.imul(x, 374_761_393) + Math.imul(y, 668_265_263);
    value = Math.imul(value ^ (value >>> 13), 1_274_126_177);
    return ((value ^ (value >>> 16)) >>> 0) / 4_294_967_295;
  }

  function noise(x: number, y: number) {
    const cellX = Math.floor(x);
    const cellY = Math.floor(y);
    const localX = smoothstep(0, 1, x - cellX);
    const localY = smoothstep(0, 1, y - cellY);
    const bottom = mix(hash(cellX, cellY), hash(cellX + 1, cellY), localX);
    const top = mix(hash(cellX, cellY + 1), hash(cellX + 1, cellY + 1), localX);
    return mix(bottom, top, localY);
  }

  function layeredNoise(x: number, y: number) {
    let value = noise(x, y) * 0.5;
    x = x * 2.03 + 1.7;
    y = y * 2.03 + 9.2;
    value += noise(x, y) * 0.25;
    x = x * 2.01 + 8.3;
    y = y * 2.01 + 2.8;
    value += noise(x, y) * 0.125;
    x = x * 2.04 + 4.1;
    y = y * 2.04 + 6.6;
    value += noise(x, y) * 0.0625;
    return value / 0.9375;
  }

  function createCloudTexture() {
    const canvas = document.createElement('canvas');
    canvas.width = TEXTURE_WIDTH;
    canvas.height = TEXTURE_HEIGHT;
    const context = canvas.getContext('2d');
    if (!context) return canvas;

    const randomValues = globalThis.crypto.getRandomValues(new Uint32Array(2));
    const seedX = ((randomValues[0] ?? 0) / 0xffffffff) * 1024;
    const seedY = ((randomValues[1] ?? 0) / 0xffffffff) * 1024;
    const image = context.createImageData(TEXTURE_WIDTH, TEXTURE_HEIGHT);
    for (let y = 0; y < TEXTURE_HEIGHT; y += 1) {
      for (let x = 0; x < TEXTURE_WIDTH; x += 1) {
        let pointX = (x / TEXTURE_WIDTH) * 4.4 + seedX;
        let pointY = (y / TEXTURE_HEIGHT) * 2.9 + seedY;
        const warpX = layeredNoise(pointX * 0.72 + 3.8, pointY * 0.72 + 1.2);
        const warpY = layeredNoise(pointX * 0.68 - 2.1, pointY * 0.68 + 7.6);
        pointX += (warpX - 0.5) * 1.15;
        pointY += (warpY - 0.5) * 1.15;

        const base = layeredNoise(pointX, pointY);
        const lightIslands = layeredNoise(pointX * 1.31 + 6.2, pointY * 1.31 - 3.7);
        const darkIslands = layeredNoise(pointX * 1.77 - 4.1, pointY * 1.77 + 8.3);
        const highlights = layeredNoise(pointX * 2.18 + 9.4, pointY * 2.18 + 2.7);

        let luminance = mix(0.145, 0.34, smoothstep(0.28, 0.73, base));
        luminance = mix(luminance, 0.61, smoothstep(0.47, 0.78, lightIslands) * 0.48);
        luminance = mix(luminance, 0.238, smoothstep(0.57, 0.82, darkIslands) * 0.42);
        luminance = mix(luminance, 0.78, smoothstep(0.72, 0.89, highlights) * 0.24);

        const channel = Math.round(Math.min(1, Math.max(0, luminance)) * 255);
        const offset = (y * TEXTURE_WIDTH + x) * 4;
        image.data[offset] = channel;
        image.data[offset + 1] = channel;
        image.data[offset + 2] = channel;
        image.data[offset + 3] = 255;
      }
    }
    context.putImageData(image, 0, 0);
    return canvas;
  }

  function getCloudTexture() {
    sharedCloudTexture ??= createCloudTexture();
    return sharedCloudTexture;
  }
</script>

<script lang="ts">
  let {
    label,
    progress = null
  }: {
    label: string;
    /** Processing progress from 0 to 1. Omit to use ambient refinement. */
    progress?: number | null;
  } = $props();

  function attachNoiseCanvas(getProgress: () => number | null) {
    return (canvas: HTMLCanvasElement) => {
      const context = canvas.getContext('2d', { alpha: false });
      if (!context) return;
      const drawingContext = context;

      const texture = getCloudTexture();
      const startedAt = performance.now();
      const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
      let timer: ReturnType<typeof setTimeout> | undefined;
      let inViewport = true;

      function effectiveProgress(now: number) {
        const suppliedProgress = getProgress();
        if (suppliedProgress !== null) {
          return Math.min(1, Math.max(0, suppliedProgress));
        }
        if (motionQuery.matches) return 0.72;
        return Math.min(1, (now - startedAt) / 1000 / AMBIENT_REFINEMENT_SECONDS);
      }

      function draw(now = performance.now()) {
        timer = undefined;
        const fidelity = effectiveProgress(now);
        const refinementLevel = Math.min(
          MAX_REFINEMENT_LEVEL,
          Math.floor(fidelity * (MAX_REFINEMENT_LEVEL + 1))
        );
        const refinementScale = 2 ** refinementLevel;
        const bounds = canvas.getBoundingClientRect();
        const aspectRatio = bounds.width > 0 ? bounds.height / bounds.width : 9 / 16;
        const columns = 8 * refinementScale;
        const rows = Math.max(1, Math.round(8 * aspectRatio)) * refinementScale;

        if (canvas.width !== columns || canvas.height !== rows) {
          canvas.width = columns;
          canvas.height = rows;
        }

        const sourceWidth = SOURCE_VIEW_WIDTH;
        const sourceHeight = Math.min(TEXTURE_HEIGHT - 16, sourceWidth * aspectRatio);
        const travelX = TEXTURE_WIDTH - sourceWidth;
        const travelY = TEXTURE_HEIGHT - sourceHeight;
        const elapsed = motionQuery.matches ? 2.4 : (now - startedAt) / 1000;
        const sourceX = travelX * (0.5 + Math.sin(elapsed * 0.071) * 0.36);
        const sourceY = travelY * (0.5 + Math.cos(elapsed * 0.053) * 0.36);

        drawingContext.imageSmoothingEnabled = true;
        drawingContext.drawImage(
          texture,
          sourceX,
          sourceY,
          sourceWidth,
          sourceHeight,
          0,
          0,
          columns,
          rows
        );

        if (!motionQuery.matches && inViewport && !document.hidden) {
          const interval = fidelity >= 1 ? RESOLVED_FRAME_INTERVAL : ACTIVE_FRAME_INTERVAL;
          timer = setTimeout(draw, interval);
        }
      }

      function scheduleDraw() {
        if (timer === undefined && inViewport && !document.hidden) {
          timer = setTimeout(draw, 0);
        }
      }

      function pause() {
        if (timer !== undefined) clearTimeout(timer);
        timer = undefined;
      }

      function handleVisibilityChange() {
        if (document.hidden) pause();
        else scheduleDraw();
      }

      function handleMotionChange() {
        pause();
        scheduleDraw();
      }

      const resizeObserver = new ResizeObserver(scheduleDraw);
      const intersectionObserver = new IntersectionObserver(([entry]) => {
        inViewport = entry?.isIntersecting ?? true;
        if (inViewport) scheduleDraw();
        else pause();
      });

      resizeObserver.observe(canvas);
      intersectionObserver.observe(canvas);
      document.addEventListener('visibilitychange', handleVisibilityChange);
      motionQuery.addEventListener('change', handleMotionChange);
      scheduleDraw();

      $effect(() => {
        getProgress();
        scheduleDraw();
      });

      return () => {
        pause();
        resizeObserver.disconnect();
        intersectionObserver.disconnect();
        document.removeEventListener('visibilitychange', handleVisibilityChange);
        motionQuery.removeEventListener('change', handleMotionChange);
      };
    };
  }
</script>

<div class="relative h-full w-full overflow-hidden bg-[#131219]" role="status" aria-live="polite">
  <canvas
    {@attach attachNoiseCanvas(() => progress)}
    aria-hidden="true"
    class="block h-full w-full bg-[linear-gradient(135deg,#202020_0%,#515151_48%,#929292_100%)] [image-rendering:pixelated]"
  ></canvas>

  <div
    class="pointer-events-none absolute inset-0 bg-[linear-gradient(180deg,transparent_55%,#1312198f_100%)]"
  >
    <div class="absolute right-3 bottom-3 left-3 flex items-center">
      <span
        class="rounded border border-white/10 bg-black/30 px-2.5 py-1.5 text-sm text-white/85 backdrop-blur-sm"
      >
        {label}
      </span>
    </div>
  </div>
</div>
