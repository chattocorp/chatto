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
 *
 * `iMouse` follows Shadertoy: `xy` is the pointer position while a button is
 * down and keeps the last position after release. `zw` is the press position.
 * `z` is negative after release, and `w` is negative after the first frame of
 * the press. Positions are render pixels with the origin at the bottom left.
 */

/** Fixed render size. It bounds GPU work independently of the display size. */
export const SHADER_WIDTH = 640;
export const SHADER_HEIGHT = 360;

/**
 * GPU time that one animation frame may use across all playing previews. The
 * preview that exceeds it pauses, so several heavy shaders cannot add up.
 */
const FRAME_BUDGET_MS = 100;
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
  /** A frame used up the frame budget. The player stops and its owner shows it as paused. */
  onslow: () => void;
};

/** Playback state for one preview canvas. */
class Player {
  #target: CanvasRenderingContext2D;
  #time = 0;
  #frame = 0;
  #last: number | undefined;
  #playing = false;
  #mouse: [number, number, number, number] = [0, 0, 0, 0];
  #events: ShaderPlayerEvents;

  constructor(
    readonly source: string,
    target: CanvasRenderingContext2D,
    events: ShaderPlayerEvents
  ) {
    this.#target = target;
    this.#events = events;
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

  /** Apply pointer input in render pixels. A paused preview draws the change at once. */
  pointer(kind: PointerKind, x: number, y: number) {
    const mouse = this.#mouse;
    if (kind === 'press') {
      this.#mouse = [x, y, x, y];
    } else if (kind === 'move') {
      if (mouse[2] <= 0) return;
      mouse[0] = x;
      mouse[1] = y;
    } else {
      if (mouse[2] <= 0) return;
      mouse[2] = -mouse[2];
    }
    if (!this.#playing) this.check(this.draw());
  }

  /**
   * Draw one frame at the animation time `now`, or again at the current time.
   * Returns the GPU time in milliseconds, or null when the shader failed.
   */
  draw(now?: number): number | null {
    const delta = now === undefined || this.#last === undefined ? 0 : (now - this.#last) / 1000;
    if (now !== undefined) this.#last = now;
    // Clamp long gaps, such as a busy main thread, so motion does not jump.
    this.#time += Math.min(delta, 0.1);
    const context_ = context();
    const entry = program(this.source);
    if (!context_ || !entry) {
      this.fail();
      return null;
    }
    const started = performance.now();
    lastSource = this.source;
    context_.viewport(0, 0, SHADER_WIDTH, SHADER_HEIGHT);
    context_.useProgram(entry.program);
    context_.uniform3f(entry.resolution, SHADER_WIDTH, SHADER_HEIGHT, 1);
    context_.uniform1f(entry.time, this.#time);
    context_.uniform1f(entry.timeDelta, delta);
    context_.uniform1i(entry.frame, this.#frame++);
    context_.uniform4fv(entry.mouse, this.#mouse);
    // Shadertoy reports a positive `w` only on the first frame of a press.
    this.#mouse[3] = -Math.abs(this.#mouse[3]);
    context_.drawArrays(context_.TRIANGLES, 0, 3);
    // Wait for the GPU, so the measured time includes the shader's own work.
    context_.readPixels(0, 0, 1, 1, context_.RGBA, context_.UNSIGNED_BYTE, pixel);
    if (context_.isContextLost()) {
      this.fail();
      return null;
    }
    this.#target.drawImage(context_.canvas, 0, 0);
    return performance.now() - started;
  }

  /** Pause after a single frame that used up the frame budget. */
  check(elapsed: number | null) {
    if (elapsed !== null && elapsed > FRAME_BUDGET_MS) this.slow();
  }

  slow() {
    this.setPlaying(false);
    this.#events.onslow();
  }

  fail() {
    this.setPlaying(false);
    this.#events.onerror();
  }
}

/** Shadertoy mouse phases: a button press, a move while pressed, and the release. */
export type PointerKind = 'press' | 'move' | 'release';

const pixel = new Uint8Array(4);
const active = new Set<Player>();
let scheduled = false;

function schedule() {
  if (scheduled || !active.size) return;
  scheduled = true;
  requestAnimationFrame(tick);
}

/** Draw all playing previews. Pause each preview that takes the total over budget. */
function tick(now: number) {
  scheduled = false;
  let spent = 0;
  for (const player of [...active]) {
    const elapsed = player.draw(now);
    if (elapsed === null) continue;
    spent += elapsed;
    if (spent > FRAME_BUDGET_MS) player.slow();
  }
  schedule();
}

/** Controls that a mounted preview uses. */
export type ShaderPlayer = {
  setPlaying(playing: boolean): void;
  /**
   * Report pointer input as fractions of the canvas size, measured from its
   * top-left corner. The player converts them to Shadertoy `iMouse` pixels.
   */
  pointer(kind: PointerKind, u: number, v: number): void;
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
  player.check(player.draw());
  return {
    setPlaying: (playing) => player.setPlaying(playing),
    pointer: (kind, u, v) =>
      player.pointer(
        kind,
        Math.min(Math.max(u, 0), 1) * SHADER_WIDTH,
        (1 - Math.min(Math.max(v, 0), 1)) * SHADER_HEIGHT
      ),
    dispose: () => player.setPlaying(false)
  };
}
