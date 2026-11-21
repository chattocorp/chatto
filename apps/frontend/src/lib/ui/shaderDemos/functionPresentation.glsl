// Function presentation, the unhinged edition. Press and drag to play:
//   1. sine:    drag sideways to set the frequency k
//   2. Fourier: drag sideways to scrub the number of harmonics
//   3. spring:  press to pluck the string where you touch
//   4. Julia:   drag to move c through the complex plane
// Labels use the font texture in iChannel0.
#define PI 3.14159265
const float SCENE = 7.0;    // seconds per slide
const float ZOOM = 3.6;     // world units across the screen height
const float ADVANCE = 0.42; // monospace advance in cells

// Per-frame state shared by the helpers.
bool pressed;
vec2 mouse;      // pointer in world units
float px;        // world units per pixel
float k;         // sine frequency
float harmonics; // Fourier terms
float pluck;     // spring origin
float kick;      // 0..1 glitch pulse
vec2 c;          // Julia constant

float hash(float n) { return fract(sin(n * 127.1) * 43758.5453); }
float hash2(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }

// Functions on show

float fourierSquare(float x, float n) {
  float sum = 0.0;
  for (int i = 0; i < 24; i++) {
    float h = float(2 * i + 1);
    sum += clamp(n - float(i), 0.0, 1.0) * sin(h * x) / h;
  }
  return sum * 4.0 / PI;
}

float f(int scene, float x, float t) {
  if (scene == 0) return sin(k * x + t * 1.5);
  if (scene == 1) return fourierSquare(x, harmonics);
  float s = abs(x - pluck);
  return 1.4 * exp(-0.45 * s) * cos(5.0 * s - t * 4.0);
}

// Antialiased curve: distance to the curve along its normal.
float plot(vec2 p, int scene, float t) {
  float e = 0.01;
  float slope = (f(scene, p.x + e, t) - f(scene, p.x - e, t)) / (2.0 * e);
  float d = abs(p.y - f(scene, p.x, t)) / sqrt(1.0 + slope * slope);
  return 1.0 - smoothstep(px, px * 2.5, d);
}

float grid(vec2 p, float spacing) {
  vec2 g = abs(fract(p / spacing + 0.5) - 0.5) * spacing;
  return 1.0 - smoothstep(0.0, px * 1.5, min(g.x, g.y));
}

vec3 julia(vec2 z, float t) {
  float i;
  for (i = 0.0; i < 96.0; i++) {
    z = vec2(z.x * z.x - z.y * z.y, 2.0 * z.x * z.y) + c;
    if (dot(z, z) > 64.0) break;
  }
  if (i >= 96.0) return vec3(0.02, 0.0, 0.05);
  float smoothed = i - log2(log2(dot(z, z))) + 4.0;
  return 0.5 + 0.5 * cos(3.0 + smoothed * 0.15 + t * 2.0 + vec3(0.0, 0.6, 1.0));
}

// Text

// ASCII glyph ch for q in its 0..1 cell; g is one pixel in texture space.
float glyph(vec2 q, int ch, float g) {
  if (q.x < 0.0 || q.x > 1.0 || q.y < 0.0 || q.y > 1.0) return 0.0;
  vec2 uv = (q + vec2(float(ch % 16), float(15 - ch / 16))) / 16.0;
  return textureGrad(iChannel0, uv, vec2(g, 0.0), vec2(0.0, g)).r;
}

// Zero-terminated string; origin is the bottom-left of its first cell.
float text(vec2 p, vec2 origin, float size, int s[16]) {
  vec2 q = (p - origin) / size;
  if (q.y < 0.0 || q.y > 1.0 || q.x < -0.5 || q.x > 16.0 * ADVANCE + 1.0) return 0.0;
  float g = px / size / 16.0;
  float ink = 0.0;
  for (int i = 0; i < 16; i++) {
    if (s[i] == 0) break;
    ink = max(ink, glyph(q - vec2(float(i) * ADVANCE, 0.0), s[i], g));
  }
  return ink;
}

int pow10(int n) { return n == 0 ? 1 : n == 1 ? 10 : n == 2 ? 100 : 1000; }

// Left-aligned number with fixed decimals and an optional plus sign.
float number(vec2 p, vec2 origin, float size, float v, int decimals, bool plus) {
  vec2 q = (p - origin) / size;
  if (q.y < 0.0 || q.y > 1.0 || q.x < -0.5 || q.x > 8.0 * ADVANCE + 1.0) return 0.0;
  float g = px / size / 16.0;
  float ink = 0.0;
  float at = 0.0;
  if (v < 0.0 || plus) {
    ink = max(ink, glyph(q, v < 0.0 ? 45 : 43, g));
    at += ADVANCE;
  }
  int scaled = int(abs(v) * float(pow10(decimals)) + 0.5);
  int whole = scaled / pow10(decimals);
  int digits = whole >= 100 ? 3 : whole >= 10 ? 2 : 1;
  for (int i = digits - 1; i >= 0; i--) {
    ink = max(ink, glyph(q - vec2(at, 0.0), 48 + (whole / pow10(i)) % 10, g));
    at += ADVANCE;
  }
  if (decimals > 0) {
    ink = max(ink, glyph(q - vec2(at, 0.0), 46, g));
    at += ADVANCE;
    for (int i = decimals - 1; i >= 0; i--) {
      ink = max(ink, glyph(q - vec2(at, 0.0), 48 + (scaled / pow10(i)) % 10, g));
      at += ADVANCE;
    }
  }
  return ink;
}

// One slide

vec3 render(vec2 p, int scene, float t, float fade) {
  vec3 accent = scene == 0 ? vec3(0.3, 0.8, 1.0)
              : scene == 1 ? vec3(1.0, 0.55, 0.25)
              : scene == 2 ? vec3(0.55, 1.0, 0.5)
                           : vec3(1.0, 0.4, 0.9);
  vec3 col = vec3(0.04, 0.05, 0.09);

  if (scene == 3) {
    col = mix(col, julia(p * 0.55, t), fade);
    float d = length(p - c / 0.55);
    col = mix(col, vec3(1.0), 1.0 - smoothstep(0.05, 0.05 + px * 2.0, abs(d - 0.08)));
  } else {
    col = mix(col, vec3(0.10, 0.12, 0.18), grid(p, 0.5) * 0.6);
    float axes = 1.0 - smoothstep(px * 0.5, px * 2.0, min(abs(p.x), abs(p.y)));
    col = mix(col, vec3(0.35, 0.40, 0.50), axes);
    if (scene == 1) {
      float target = 1.0 - smoothstep(px, px * 2.5, abs(p.y - sign(sin(p.x))));
      col = mix(col, accent * 0.5, target * step(0.5, fract(p.x * 4.0)) * fade);
    }
    float reveal = pressed ? 4.0 : mix(-3.4, 3.4, smoothstep(0.2, 2.0, t));
    float shown = step(p.x, reveal) * fade;
    col = mix(col, accent, plot(p, scene, t) * shown);
    col += accent * 0.2 * exp(-abs(p.y - f(scene, p.x, t)) * 10.0) * shown;
    float x = pressed ? mouse.x : t < 2.0 ? reveal : -3.0 + mod((t - 2.0) * 1.5, 6.0);
    float d = length(p - vec2(x, f(scene, x, t)));
    col += accent * (1.0 - smoothstep(0.035, 0.035 + px * 2.0, d)) * fade;
    col += accent * 0.5 * exp(-d * 8.0) * fade;
  }

  // Title and live readout.
  vec2 at = vec2(-3.1, 1.25);
  vec2 readout = vec2(-3.1, 0.88);
  float size = 0.42;
  float small = 0.32;
  float label = 0.0;
  if (scene == 0) {
    label = max(text(p, at, size, int[16](121, 32, 61, 32, 115, 105, 110, 40, 107, 120, 32, 43, 32, 116, 41, 0)), text(p, readout, small, int[16](107, 32, 61, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)));
    label = max(label, number(p, readout + vec2(4.0 * ADVANCE * small, 0.0), small, k, 2, false));
  } else if (scene == 1) {
    label = max(text(p, at, size, int[16](70, 111, 117, 114, 105, 101, 114, 32, 115, 101, 114, 105, 101, 115, 0, 0)), text(p, readout, small, int[16](110, 32, 61, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)));
    label = max(label, number(p, readout + vec2(4.0 * ADVANCE * small, 0.0), small, ceil(harmonics), 0, false));
  } else if (scene == 2) {
    label = max(text(p, at, size, int[16](68, 97, 109, 112, 101, 100, 32, 115, 112, 114, 105, 110, 103, 0, 0, 0)), text(p, readout, small, int[16](120, 48, 32, 61, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)));
    label = max(label, number(p, readout + vec2(5.0 * ADVANCE * small, 0.0), small, pluck, 2, false));
  } else {
    label = max(text(p, at, size, int[16](122, 32, 45, 62, 32, 122, 94, 50, 32, 43, 32, 99, 0, 0, 0, 0)), text(p, readout, small, int[16](99, 32, 61, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)));
    label = max(label, number(p, readout + vec2(4.0 * ADVANCE * small, 0.0), small, c.x, 2, false));
    label = max(label, number(p, readout + vec2(10.0 * ADVANCE * small, 0.0), small, c.y, 2, true));
    label = max(label, glyph((p - readout) / small - vec2(15.0 * ADVANCE, 0.0), 105, px / small / 16.0));
  }
  // Dark panel keeps the labels readable over the fractal.
  vec2 box = abs(p - vec2(-2.0, 1.25)) - vec2(1.25, 0.45);
  if (scene == 3) col *= 1.0 - 0.75 * smoothstep(0.1, 0.0, max(box.x, box.y)) * fade;
  col = mix(col, vec3(0.95, 0.96, 1.0), label * fade);

  // Invitation.
  if (!pressed) {
    float blink = 0.55 + 0.45 * sin(iTime * 5.0);
    col = mix(col, accent, text(p, vec2(-3.1, -1.75), 0.26, int[16](100, 114, 97, 103, 32, 109, 101, 33, 0, 0, 0, 0, 0, 0, 0, 0)) * blink);
  }

  // Slide indicator.
  for (int i = 0; i < 4; i++) {
    float r = length(p - vec2((float(i) - 1.5) * 0.3, -1.6));
    float on = i == scene ? 1.0 : 0.3;
    col = mix(col, vec3(0.9) * on, 1.0 - smoothstep(0.045, 0.045 + px * 1.5, r));
  }
  return col;
}

// Screen

void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  int scene = int(mod(floor(iTime / SCENE), 4.0));
  float t = mod(iTime, SCENE);
  float fade = smoothstep(0.0, 0.4, t) * (1.0 - smoothstep(SCENE - 0.4, SCENE, t));

  pressed = iMouse.z > 0.0;
  mouse = (iMouse.xy - 0.5 * iResolution.xy) / iResolution.y * ZOOM;
  px = ZOOM / iResolution.y;
  float mx = iMouse.x / iResolution.x;
  k = pressed ? mix(0.3, 6.0, mx) : 1.0 + 0.5 * sin(t * 0.7);
  harmonics = pressed ? mix(1.0, 24.0, mx) : 1.0 + t * 2.5;
  pluck = pressed ? mouse.x : -3.2;
  c = pressed ? mouse * 0.55 : 0.7885 * vec2(cos(t * 0.6), sin(t * 0.6));

  // Harmonics, slide changes and touches shake the screen.
  kick = scene == 1 ? exp(-fract(harmonics) * 6.0) * 0.6 : 0.0;
  kick = max(kick, 1.0 - fade);
  if (pressed) kick = max(kick, 0.2);

  // Curved CRT glass, glitch slices, and shake.
  vec2 uv = fragCoord / iResolution.xy - 0.5;
  uv *= 1.0 + 0.12 * dot(uv, uv);
  float band = floor(uv.y * 40.0) + floor(iTime * 24.0);
  uv.x += (hash(band) - 0.5) * 0.1 * kick * step(0.65, hash(band + 7.0));
  uv += (vec2(hash(iTime), hash(iTime + 3.1)) - 0.5) * 0.012 * kick;
  vec2 p = uv * vec2(iResolution.x / iResolution.y, 1.0) * ZOOM;

  // Chromatic aberration.
  vec2 split = uv * (0.04 + 0.3 * kick);
  vec3 col = vec3(render(p + split, scene, t, fade).r,
                  render(p, scene, t, fade).g,
                  render(p - split, scene, t, fade).b);

  col *= 0.9 + 0.1 * sin(fragCoord.y * PI);              // scanlines
  col *= 1.0 - 1.2 * dot(uv, uv);                        // vignette
  col += (hash2(fragCoord + fract(iTime) * 100.0) - 0.5) * 0.06; // grain
  if (scene == 1) col += vec3(1.0, 0.85, 0.7) * kick * 0.12; // harmonic flash
  if (abs(uv.x) > 0.5 || abs(uv.y) > 0.5) col = vec3(0.0); // outside the glass
  fragColor = vec4(col, 1.0);
}
