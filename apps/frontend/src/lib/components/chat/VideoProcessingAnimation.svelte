<!--
	Animated placeholder for videos while the server prepares playback variants.
	The WebGL canvas is decorative; the live status text remains accessible.
-->
<script lang="ts">
  let {
    label,
    progress = null
  }: {
    label: string;
    /** Processing progress from 0 to 1. Omit to use the ambient refinement cycle. */
    progress?: number | null;
  } = $props();

  const vertexShaderSource = `
		attribute vec2 a_position;

		void main() {
			gl_Position = vec4(a_position, 0.0, 1.0);
		}
	`;

  const fragmentShaderSource = `
		precision highp float;

		uniform vec2 u_resolution;
		uniform float u_time;
		uniform float u_progress;

		float ink = 0.055;
		float charcoal = 0.145;
		float graphite = 0.34;
		float silver = 0.61;
		float highlight = 0.78;

		float hash(vec2 value) {
			return fract(sin(dot(value, vec2(127.1, 311.7))) * 43758.5453);
		}

		float noise(vec2 point) {
			vec2 cell = floor(point);
			vec2 local = fract(point);
			local = local * local * (3.0 - 2.0 * local);

			float bottom = mix(hash(cell), hash(cell + vec2(1.0, 0.0)), local.x);
			float top = mix(
				hash(cell + vec2(0.0, 1.0)),
				hash(cell + vec2(1.0, 1.0)),
				local.x
			);
			return mix(bottom, top, local.y);
		}

		float layeredNoise(vec2 point) {
			float value = noise(point) * 0.5;
			point = point * 2.03 + vec2(1.7, 9.2);
			value += noise(point) * 0.25;
			point = point * 2.01 + vec2(8.3, 2.8);
			value += noise(point) * 0.125;
			point = point * 2.04 + vec2(4.1, 6.6);
			value += noise(point) * 0.0625;
			return value / 0.9375;
		}

		float picture(vec2 uv, float time) {
			vec2 centered = uv * 2.0 - 1.0;
			vec2 point = uv * vec2(3.4, 2.35);
			vec2 drift = vec2(time * 0.021, -time * 0.017);
			vec2 warp = vec2(
				layeredNoise(point * 0.72 + drift + vec2(3.8, 1.2)),
				layeredNoise(point * 0.68 - drift + vec2(-2.1, 7.6))
			);
			point += (warp - 0.5) * 1.15;

			float base = layeredNoise(point + drift);
			float coolIslands = layeredNoise(point * 1.31 + vec2(6.2, -3.7) - drift * 0.8);
			float greenIslands = layeredNoise(point * 1.77 + vec2(-4.1, 8.3) + drift * 0.6);
			float warmFlecks = layeredNoise(point * 2.18 + vec2(9.4, 2.7) - drift * 0.45);

			float luminance = mix(charcoal, graphite, smoothstep(0.28, 0.73, base));
			luminance = mix(
				luminance,
				silver,
				smoothstep(0.47, 0.78, coolIslands) * 0.48
			);
			luminance = mix(
				luminance,
				graphite * 0.7,
				smoothstep(0.57, 0.82, greenIslands) * 0.42
			);
			luminance = mix(
				luminance,
				highlight,
				smoothstep(0.72, 0.89, warmFlecks) * 0.24
			);
			float vignette = smoothstep(1.2, 0.24, length(centered * vec2(0.75, 0.9)));
			luminance = mix(ink, luminance, 0.78 + vignette * 0.22);
			return luminance;
		}

		void main() {
			vec2 uv = gl_FragCoord.xy / u_resolution.xy;
			float fidelity = clamp(u_progress, 0.0, 1.0);
			// Each stage subdivides every tile into four, keeping the grid boundaries nested.
			float refinementLevel = min(5.0, floor(fidelity * 6.0));
			float refinementScale = exp2(refinementLevel);
			float baseRows = max(1.0, floor(8.0 * u_resolution.y / u_resolution.x + 0.5));
			vec2 sampleGrid = vec2(8.0, baseRows) * refinementScale;
			vec2 block = floor(uv * sampleGrid);
			vec2 sampleUv = (block + 0.5) / sampleGrid;

			float instability = 1.0 - fidelity;
			float signalOffset = 0.014 * instability;
			float centre = picture(sampleUv, u_time);
			float shiftedRight = picture(sampleUv + vec2(signalOffset, 0.0), u_time);
			float shiftedLeft = picture(sampleUv - vec2(signalOffset, 0.0), u_time);
			float luminance = centre * 0.84 + shiftedRight * 0.08 + shiftedLeft * 0.08;

			float blockNoise =
				(hash(block + floor(u_time * 3.0)) - 0.5) * 0.18 * instability;
			luminance += blockNoise;

			float scanline = sin(gl_FragCoord.y * 3.14159) * 0.008;
			luminance += scanline;

			gl_FragColor = vec4(vec3(luminance), 1.0);
		}
	`;

  function compileShader(
    gl: WebGLRenderingContext,
    type: number,
    source: string
  ): WebGLShader | null {
    const shader = gl.createShader(type);
    if (!shader) return null;

    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (gl.getShaderParameter(shader, gl.COMPILE_STATUS)) return shader;

    gl.deleteShader(shader);
    return null;
  }

  function attachCheckerboard(getProgress: () => number | null) {
    return (canvas: HTMLCanvasElement) => {
      const context = canvas.getContext('webgl', {
        alpha: false,
        antialias: false,
        powerPreference: 'low-power'
      });
      if (!context) return;
      const gl: WebGLRenderingContext = context;

      const vertexShader = compileShader(gl, gl.VERTEX_SHADER, vertexShaderSource);
      const fragmentShader = compileShader(gl, gl.FRAGMENT_SHADER, fragmentShaderSource);
      if (!vertexShader || !fragmentShader) {
        if (vertexShader) gl.deleteShader(vertexShader);
        if (fragmentShader) gl.deleteShader(fragmentShader);
        return;
      }

      const program = gl.createProgram();
      const buffer = gl.createBuffer();
      if (!program || !buffer) {
        gl.deleteShader(vertexShader);
        gl.deleteShader(fragmentShader);
        if (program) gl.deleteProgram(program);
        if (buffer) gl.deleteBuffer(buffer);
        return;
      }

      gl.attachShader(program, vertexShader);
      gl.attachShader(program, fragmentShader);
      gl.linkProgram(program);
      if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
        gl.deleteBuffer(buffer);
        gl.deleteProgram(program);
        gl.deleteShader(vertexShader);
        gl.deleteShader(fragmentShader);
        return;
      }

      const positionLocation = gl.getAttribLocation(program, 'a_position');
      const resolutionLocation = gl.getUniformLocation(program, 'u_resolution');
      const timeLocation = gl.getUniformLocation(program, 'u_time');
      const progressLocation = gl.getUniformLocation(program, 'u_progress');
      const startedAt = performance.now();
      const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
      let animationFrame: number | undefined;
      let inViewport = true;

      gl.useProgram(program);
      gl.bindBuffer(gl.ARRAY_BUFFER, buffer);
      gl.bufferData(
        gl.ARRAY_BUFFER,
        new Float32Array([-1, -1, 1, -1, -1, 1, -1, 1, 1, -1, 1, 1]),
        gl.STATIC_DRAW
      );
      gl.enableVertexAttribArray(positionLocation);
      gl.vertexAttribPointer(positionLocation, 2, gl.FLOAT, false, 0, 0);

      function draw(now = startedAt) {
        animationFrame = undefined;
        const elapsed = motionQuery.matches ? 2.4 : (now - startedAt) / 1000;
        const suppliedProgress = getProgress();
        const ambientProgress = motionQuery.matches ? 0.72 : Math.min(1, elapsed / 14);
        const effectiveProgress =
          suppliedProgress === null ? ambientProgress : Math.min(1, Math.max(0, suppliedProgress));

        gl.uniform2f(resolutionLocation, canvas.width, canvas.height);
        gl.uniform1f(timeLocation, elapsed);
        gl.uniform1f(progressLocation, effectiveProgress);
        gl.drawArrays(gl.TRIANGLES, 0, 6);

        if (!motionQuery.matches && inViewport && !document.hidden) {
          animationFrame = requestAnimationFrame(draw);
        }
      }

      function scheduleDraw() {
        if (animationFrame === undefined && inViewport && !document.hidden) {
          animationFrame = requestAnimationFrame(draw);
        }
      }

      function resize() {
        const bounds = canvas.getBoundingClientRect();
        const pixelRatio = Math.min(window.devicePixelRatio || 1, 2);
        const width = Math.max(1, Math.round(bounds.width * pixelRatio));
        const height = Math.max(1, Math.round(bounds.height * pixelRatio));
        if (canvas.width === width && canvas.height === height) return;

        canvas.width = width;
        canvas.height = height;
        gl.viewport(0, 0, width, height);
        scheduleDraw();
      }

      function handleVisibilityChange() {
        if (document.hidden) {
          if (animationFrame !== undefined) cancelAnimationFrame(animationFrame);
          animationFrame = undefined;
        } else {
          scheduleDraw();
        }
      }

      function handleMotionChange() {
        if (motionQuery.matches && animationFrame !== undefined) {
          cancelAnimationFrame(animationFrame);
          animationFrame = undefined;
        }
        scheduleDraw();
      }

      const resizeObserver = new ResizeObserver(resize);
      const intersectionObserver = new IntersectionObserver(([entry]) => {
        inViewport = entry?.isIntersecting ?? true;
        if (inViewport) {
          scheduleDraw();
        } else if (animationFrame !== undefined) {
          cancelAnimationFrame(animationFrame);
          animationFrame = undefined;
        }
      });

      resizeObserver.observe(canvas);
      intersectionObserver.observe(canvas);
      document.addEventListener('visibilitychange', handleVisibilityChange);
      motionQuery.addEventListener('change', handleMotionChange);
      resize();
      scheduleDraw();

      $effect(() => {
        getProgress();
        scheduleDraw();
      });

      return () => {
        if (animationFrame !== undefined) cancelAnimationFrame(animationFrame);
        resizeObserver.disconnect();
        intersectionObserver.disconnect();
        document.removeEventListener('visibilitychange', handleVisibilityChange);
        motionQuery.removeEventListener('change', handleMotionChange);
        gl.deleteBuffer(buffer);
        gl.deleteProgram(program);
        gl.deleteShader(vertexShader);
        gl.deleteShader(fragmentShader);
      };
    };
  }
</script>

<div class="relative h-full w-full overflow-hidden bg-[#131219]" role="status" aria-live="polite">
  <canvas
    {@attach attachCheckerboard(() => progress)}
    aria-hidden="true"
    class="block h-full w-full bg-[linear-gradient(135deg,#202020_0%,#515151_48%,#929292_100%)]"
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
