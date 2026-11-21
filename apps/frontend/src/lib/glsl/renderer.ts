/**
 * Shared WebGL renderer for GLSL fragment shader previews in Markdown.
 *
 * Browsers limit the number of live WebGL contexts per page. All previews
 * therefore share one offscreen context. Each frame renders into it and is then
 * copied into the preview's own 2D canvas. One animation loop draws every
 * playing preview.
 *
 * Shaders use the Shadertoy entry point
 * `void mainImage(out vec4 fragColor, in vec2 fragCoord)` with the uniforms
 * `iResolution`, `iTime`, `iTimeDelta`, `iFrame`, and `iMouse`. Textures,
 * buffers, and audio inputs are not supported.
 */

/** Fixed render size. It bounds GPU work independently of the display size. */
export const SHADER_WIDTH = 640;
export const SHADER_HEIGHT = 360;

/** A frame that takes longer than this pauses its preview. */
const SLOW_FRAME_MS = 100;
/** Keep a bounded number of linked programs. */
const MAX_PROGRAMS = 16;

const VERTEX_SOURCE = `#version 300 es
void main() {
  vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);
}`;

function fragmentSource(source: string): string {
  return `#version 300 es
precision highp float;
precision highp int;
uniform vec3 iResolution;
uniform float iTime;
uniform float iTimeDelta;
uniform int iFrame;
uniform vec4 iMouse;
out vec4 chattoFragColor;
#line 1
${source}
void main() {
  vec4 color = vec4(0.0, 0.0, 0.0, 1.0);
  mainImage(color, gl_FragCoord.xy);
  chattoFragColor = vec4(color.rgb, 1.0);
}`;
}

type Program = {
  program: WebGLProgram;
  resolution: WebGLUniformLocation | null;
  time: WebGLUniformLocation | null;
  timeDelta: WebGLUniformLocation | null;
  frame: WebGLUniformLocation | null;
  mouse: WebGLUniformLocation | null;
};

let gl: WebGL2RenderingContext | null = null;
/** Source of the latest draw, which receives the blame for a context loss. */
let lastSource: string | undefined;
// Insertion order gives least-recently-used eviction; `null` records a failure.
const programs = new Map<string, Program | null>();

/**
 * Forget a lost context and its programs. The GPU watchdog resets a context
 * after a hung draw. Keep the shader that drew last marked as failed, so a new
 * context does not run it again.
 */
function lose(lost: WebGL2RenderingContext) {
  if (gl !== lost) return;
  programs.clear();
  if (lastSource !== undefined) programs.set(lastSource, null);
  gl = null;
}

function context(): WebGL2RenderingContext | null {
  if (gl?.isContextLost()) lose(gl);
  if (gl) return gl;
  const canvas = document.createElement('canvas');
  canvas.width = SHADER_WIDTH;
  canvas.height = SHADER_HEIGHT;
  const next = canvas.getContext('webgl2', {
    alpha: false,
    antialias: false,
    depth: false,
    stencil: false,
    powerPreference: 'low-power'
  });
  if (!next) return null;
  canvas.addEventListener('webglcontextlost', () => lose(next));
  gl = next;
  return gl;
}

function compile(context: WebGL2RenderingContext, type: number, source: string) {
  const shader = context.createShader(type);
  if (!shader) return null;
  context.shaderSource(shader, source);
  context.compileShader(shader);
  if (context.getShaderParameter(shader, context.COMPILE_STATUS)) return shader;
  context.deleteShader(shader);
  return null;
}

function program(source: string): Program | null {
  const cached = programs.get(source);
  if (cached !== undefined) {
    programs.delete(source);
    programs.set(source, cached);
    return cached;
  }
  const context_ = context();
  if (!context_) return null;
  const vertex = compile(context_, context_.VERTEX_SHADER, VERTEX_SOURCE);
  const fragment = compile(context_, context_.FRAGMENT_SHADER, fragmentSource(source));
  let result: Program | null = null;
  const linked = context_.createProgram();
  if (vertex && fragment && linked) {
    context_.attachShader(linked, vertex);
    context_.attachShader(linked, fragment);
    context_.linkProgram(linked);
    if (context_.getProgramParameter(linked, context_.LINK_STATUS)) {
      result = {
        program: linked,
        resolution: context_.getUniformLocation(linked, 'iResolution'),
        time: context_.getUniformLocation(linked, 'iTime'),
        timeDelta: context_.getUniformLocation(linked, 'iTimeDelta'),
        frame: context_.getUniformLocation(linked, 'iFrame'),
        mouse: context_.getUniformLocation(linked, 'iMouse')
      };
    } else {
      context_.deleteProgram(linked);
    }
  }
  if (vertex) context_.deleteShader(vertex);
  if (fragment) context_.deleteShader(fragment);
  programs.set(source, result);
  while (programs.size > MAX_PROGRAMS) {
    const [oldest, entry] = programs.entries().next().value!;
    if (entry) context_.deleteProgram(entry.program);
    programs.delete(oldest);
  }
  return result;
}

/** Compile and link the shader. Returns false for invalid source or missing WebGL 2. */
export function isShaderUsable(source: string): boolean {
  return program(source) !== null;
}

/** Callbacks that a preview receives from the shared renderer. */
export type ShaderPlayerEvents = {
  /** The shader cannot render, or it caused a context loss. */
  onerror: () => void;
  /** A frame was too slow. The player stops and its owner shows it as paused. */
  onslow: () => void;
};

/** Playback state for one preview canvas. */
class Player {
  #target: CanvasRenderingContext2D;
  #time = 0;
  #frame = 0;
  #last: number | undefined;
  #playing = false;
  #events: ShaderPlayerEvents;

  constructor(
    readonly source: string,
    target: CanvasRenderingContext2D,
    events: ShaderPlayerEvents
  ) {
    this.#target = target;
    this.#events = events;
  }

  get playing() {
    return this.#playing;
  }

  /** Start or stop the animation. Time continues from the paused position. */
  setPlaying(playing: boolean) {
    if (this.#playing === playing) return;
    this.#playing = playing;
    this.#last = undefined;
    if (playing) {
      active.add(this);
      schedule();
    } else {
      active.delete(this);
    }
  }

  /** Draw one frame. Returns false when the shader failed or was too slow. */
  draw(now?: number): boolean {
    const delta = now === undefined || this.#last === undefined ? 0 : (now - this.#last) / 1000;
    this.#last = now;
    // Clamp long gaps, such as a busy main thread, so motion does not jump.
    this.#time += Math.min(delta, 0.1);
    const context_ = context();
    const entry = program(this.source);
    if (!context_ || !entry) {
      this.fail();
      return false;
    }
    const started = performance.now();
    lastSource = this.source;
    context_.viewport(0, 0, SHADER_WIDTH, SHADER_HEIGHT);
    context_.useProgram(entry.program);
    context_.uniform3f(entry.resolution, SHADER_WIDTH, SHADER_HEIGHT, 1);
    context_.uniform1f(entry.time, this.#time);
    context_.uniform1f(entry.timeDelta, delta);
    context_.uniform1i(entry.frame, this.#frame++);
    context_.uniform4f(entry.mouse, 0, 0, 0, 0);
    context_.drawArrays(context_.TRIANGLES, 0, 3);
    // Wait for the GPU, so the measured time includes the shader's own work.
    context_.readPixels(0, 0, 1, 1, context_.RGBA, context_.UNSIGNED_BYTE, pixel);
    if (context_.isContextLost()) {
      this.fail();
      return false;
    }
    this.#target.drawImage(context_.canvas, 0, 0);
    if (performance.now() - started > SLOW_FRAME_MS) {
      this.setPlaying(false);
      this.#events.onslow();
      return false;
    }
    return true;
  }

  fail() {
    this.setPlaying(false);
    this.#events.onerror();
  }
}

const pixel = new Uint8Array(4);
const active = new Set<Player>();
let scheduled = false;

function schedule() {
  if (scheduled || !active.size) return;
  scheduled = true;
  requestAnimationFrame(tick);
}

function tick(now: number) {
  scheduled = false;
  for (const player of [...active]) player.draw(now);
  schedule();
}

/** Controls that a mounted preview uses. */
export type ShaderPlayer = {
  setPlaying(playing: boolean): void;
  /** Stop playback and release the target canvas. */
  dispose(): void;
};

/**
 * Draw the first frame of a shader into `canvas` and return its playback
 * controls. The canvas must be `SHADER_WIDTH` by `SHADER_HEIGHT` pixels.
 * Returns null, after `onerror`, when the canvas has no 2D context.
 */
export function createShaderPlayer(
  source: string,
  canvas: HTMLCanvasElement,
  events: ShaderPlayerEvents
): ShaderPlayer | null {
  const target = canvas.getContext('2d');
  if (!target) {
    events.onerror();
    return null;
  }
  const player = new Player(source, target, events);
  // A failed or slow first frame reports through `events`.
  player.draw();
  return {
    setPlaying: (playing) => player.setPlaying(playing),
    dispose: () => player.setPlaying(false)
  };
}
