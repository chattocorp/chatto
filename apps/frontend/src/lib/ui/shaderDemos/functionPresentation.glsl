// A looping presentation of three functions animated over iTime:
// 1. a sine wave whose frequency breathes,
// 2. a Fourier series that adds harmonics until it becomes a square wave,
// 3. a damped spring response travelling to the right.
// Labels use the font texture in iChannel0.
#define PI 3.14159265
const float SCENE = 6.0; // seconds per scene
const float ZOOM = 3.6;  // world units across the screen height

float fourierSquare(float x, float harmonics) {
  float sum = 0.0;
  for (int k = 0; k < 24; k++) {
    float h = float(2 * k + 1);
    // Fade each new harmonic in, so the curve morphs instead of jumping.
    sum += clamp(harmonics - float(k), 0.0, 1.0) * sin(h * x) / h;
  }
  return sum * 4.0 / PI;
}

float f(int scene, float x, float t) {
  if (scene == 0) return sin(x * (1.0 + 0.5 * sin(t * 0.7)) + t * 1.5);
  if (scene == 1) return fourierSquare(x, 1.0 + t * 2.5);
  float s = x + 3.2;
  return 1.4 * exp(-0.35 * s) * cos(5.0 * s - t * 3.0);
}

// Antialiased curve: vertical distance divided by the slope gives the true distance.
float plot(vec2 p, int scene, float t, float px) {
  float e = 0.01;
  float slope = (f(scene, p.x + e, t) - f(scene, p.x - e, t)) / (2.0 * e);
  float d = abs(p.y - f(scene, p.x, t)) / sqrt(1.0 + slope * slope);
  return 1.0 - smoothstep(px, px * 2.5, d);
}

// Coverage of ASCII code c, for p in its 0..1 cell.
float glyph(vec2 p, int c) {
  if (p.x < 0.0 || p.x > 1.0 || p.y < 0.0 || p.y > 1.0) return 0.0;
  vec2 uv = (p + vec2(float(c % 16), float(15 - c / 16))) / 16.0;
  return textureGrad(iChannel0, uv, dFdx(uv), dFdy(uv)).r;
}

const float ADVANCE = 0.42; // monospace advance, in cells

// A zero-terminated string with its first cell's bottom-left corner at origin.
float text(vec2 p, vec2 origin, float size, int s[24]) {
  vec2 q = (p - origin) / size;
  if (q.y < 0.0 || q.y > 1.0 || q.x < -0.5 || q.x > 24.0 * ADVANCE + 1.0) return 0.0;
  float ink = 0.0;
  for (int i = 0; i < 24; i++) {
    if (s[i] == 0) break;
    ink = max(ink, glyph(q - vec2(float(i) * ADVANCE, 0.0), s[i]));
  }
  return ink;
}

float number(vec2 p, vec2 origin, float size, int n) {
  vec2 q = (p - origin) / size;
  float ink = glyph(q - vec2(ADVANCE, 0.0), 48 + n % 10);
  if (n >= 10) ink = max(ink, glyph(q, 48 + n / 10));
  return ink;
}

float grid(vec2 p, float spacing, float px) {
  vec2 g = abs(fract(p / spacing + 0.5) - 0.5) * spacing;
  return 1.0 - smoothstep(0.0, px * 1.5, min(g.x, g.y));
}

void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  vec2 p = (fragCoord - 0.5 * iResolution.xy) / iResolution.y * ZOOM;
  float px = ZOOM / iResolution.y;
  int scene = int(mod(floor(iTime / SCENE), 3.0));
  float t = mod(iTime, SCENE);
  float fade = smoothstep(0.0, 0.4, t) * (1.0 - smoothstep(SCENE - 0.4, SCENE, t));
  vec3 accent = scene == 0 ? vec3(0.3, 0.8, 1.0)
              : scene == 1 ? vec3(1.0, 0.55, 0.25)
                           : vec3(0.55, 1.0, 0.5);

  // Background, grid and axes.
  vec3 col = vec3(0.04, 0.05, 0.09);
  col = mix(col, vec3(0.10, 0.12, 0.18), grid(p, 0.5, px) * 0.6);
  float axes = 1.0 - smoothstep(px * 0.5, px * 2.0, min(abs(p.x), abs(p.y)));
  col = mix(col, vec3(0.35, 0.40, 0.50), axes);

  // Scene 2 shows its target: a dashed square wave.
  if (scene == 1) {
    float target = 1.0 - smoothstep(px, px * 2.5, abs(p.y - sign(sin(p.x))));
    col = mix(col, accent * 0.5, target * step(0.5, fract(p.x * 4.0)) * fade);
  }

  // The curve draws in from the left, with a soft glow.
  float reveal = mix(-3.4, 3.4, smoothstep(0.2, 2.0, t));
  float shown = step(p.x, reveal) * fade;
  col = mix(col, accent, plot(p, scene, t, px) * shown);
  col += accent * 0.15 * exp(-abs(p.y - f(scene, p.x, t)) * 12.0) * shown;

  // A tracer dot follows the pen, then rides along the curve.
  float x = t < 2.0 ? reveal : -3.0 + mod((t - 2.0) * 1.5, 6.0);
  float d = length(p - vec2(x, f(scene, x, t)));
  col += accent * (1.0 - smoothstep(0.03, 0.03 + px * 2.0, d)) * fade;
  col += accent * 0.4 * exp(-d * 10.0) * fade;

  // Slide title.
  vec2 at = vec2(-3.1, 1.2);
  float size = 0.42;
  float label = scene == 0 ? text(p, at, size, int[24](121, 32, 61, 32, 115, 105, 110, 40, 107, 120, 32, 43, 32, 116, 41, 0, 0, 0, 0, 0, 0, 0, 0, 0))
              : scene == 1 ? text(p, at, size, int[24](70, 111, 117, 114, 105, 101, 114, 32, 115, 101, 114, 105, 101, 115, 44, 32, 110, 32, 61, 0, 0, 0, 0, 0))
                           : text(p, at, size, int[24](121, 32, 61, 32, 101, 94, 45, 120, 32, 99, 111, 115, 40, 53, 120, 32, 45, 32, 51, 116, 41, 0, 0, 0));
  if (scene == 1) {
    int n = int(clamp(ceil(1.0 + t * 2.5), 1.0, 24.0));
    label = max(label, number(p, at + vec2(20.0 * ADVANCE * size, 0.0), size, n));
  }
  col = mix(col, vec3(0.92, 0.94, 1.0), label * fade);

  // Slide indicator.
  for (int i = 0; i < 3; i++) {
    float r = length(p - vec2(float(i - 1) * 0.3, -1.55));
    float on = i == scene ? 1.0 : 0.3;
    col = mix(col, vec3(0.9) * on, 1.0 - smoothstep(0.045, 0.045 + px * 1.5, r));
  }

  fragColor = vec4(col, 1.0);
}
