// EVENT HORIZON
// A black hole bends the light of the stars and of its own accretion disk.
// Drag sideways to orbit it. Drag up to feed it. Do not feed it too much.
#define PI 3.14159265
const float ADVANCE = 0.42; // monospace advance in glyph cells
float px;                   // screen units per pixel

float hash3(vec3 p) {
  p = fract(p * 0.3183099 + 0.1) * 17.0;
  return fract(p.x * p.y * p.z * (p.x + p.y + p.z));
}
float hash1(float n) { return fract(sin(n) * 43758.5453); }

// Two layers of stars and a faint violet haze, looked up by direction.
vec3 sky(vec3 d) {
  vec3 col = vec3(0.0);
  for (int i = 0; i < 2; i++) {
    vec3 q = d * (i == 0 ? 90.0 : 210.0);
    vec3 cell = floor(q);
    float h = hash3(cell);
    if (h > 0.955) {
      vec3 star = cell + 0.5 + 0.3 * (vec3(hash3(cell + 1.0), hash3(cell + 2.0), hash3(cell + 3.0)) - 0.5);
      vec3 tint = mix(vec3(0.6, 0.7, 1.0), vec3(1.0, 0.75, 0.5), hash3(cell + 4.0));
      col += tint * exp(-length(q - star) * 7.0) * (h - 0.955) * 30.0;
    }
  }
  float haze = sin(d.x * 3.0 + sin(d.y * 4.0)) * sin(d.z * 3.5 + sin(d.x * 2.0));
  return col + vec3(0.3, 0.06, 0.4) * 0.06 * (haze * 0.5 + 0.5);
}

// Emission where a ray crosses the disk plane at p, travelling along rd.
vec3 disk(vec3 p, vec3 rd, float t) {
  float r = length(p.xz);
  float a = atan(p.z, p.x);
  float swirl = a + t * 4.0 / pow(r, 1.5); // inner orbits are faster
  float bands = 0.5 + 0.5 * sin(swirl * 2.0 + r * 2.5 + 2.0 * sin(r * 1.3 - t));
  float grain = 0.6 + 0.4 * sin(swirl * 7.0 - r * 3.0 + sin(swirl * 3.0));
  float heat = smoothstep(7.5, 2.2, r);
  vec3 col = mix(vec3(1.0, 0.22, 0.04), vec3(1.0, 0.9, 0.7), heat * heat);
  col *= (0.35 + 0.9 * bands * grain) * (0.25 + heat);
  // Doppler beaming: the side that rushes towards us is brighter.
  vec3 velocity = normalize(vec3(-p.z, 0.0, p.x));
  float beam = 1.0 + 0.75 * dot(velocity, -rd);
  return col * beam * beam * smoothstep(2.2, 2.6, r) * smoothstep(7.5, 6.0, r) * 1.5;
}

// March a ray through curved space. Gravity pulls the ray towards the hole.
vec3 trace(vec3 p, vec3 rd, float mass, float t) {
  vec3 col = vec3(0.0);
  float through = 1.0; // light that still passes the disk layers in front
  for (int i = 0; i < 150; i++) {
    float r = length(p);
    if (r < mass) return col; // past the horizon nothing returns
    float dt = clamp(0.08 * r, 0.03, 0.7);
    rd = normalize(rd - p * (1.5 * mass * dt / (r * r * r)));
    vec3 next = p + rd * dt;
    if (p.y * next.y < 0.0) {
      vec3 hit = mix(p, next, p.y / (p.y - next.y));
      float hr = length(hit.xz);
      if (hr > 2.2 && hr < 7.5) {
        col += disk(hit, rd, t) * through;
        through *= 0.3;
      }
    }
    p = next;
    if (r > 40.0) break;
  }
  return col + through * sky(rd);
}

// Text from the font texture in iChannel0.
float glyph(vec2 q, int ch, float g) {
  if (q.x < 0.0 || q.x > 1.0 || q.y < 0.0 || q.y > 1.0) return 0.0;
  vec2 uv = (q + vec2(float(ch % 16), float(15 - ch / 16))) / 16.0;
  return textureGrad(iChannel0, uv, vec2(g, 0.0), vec2(0.0, g)).r;
}

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

// Positive number with two decimals, left-aligned.
float number(vec2 p, vec2 origin, float size, float v) {
  vec2 q = (p - origin) / size;
  float g = px / size / 16.0;
  int n = int(v * 100.0 + 0.5);
  float ink = glyph(q, 48 + (n / 100) % 10, g);
  ink = max(ink, glyph(q - vec2(ADVANCE, 0.0), 46, g));
  ink = max(ink, glyph(q - vec2(2.0 * ADVANCE, 0.0), 48 + (n / 10) % 10, g));
  return max(ink, glyph(q - vec2(3.0 * ADVANCE, 0.0), 48 + n % 10, g));
}

void mainImage(out vec4 fragColor, in vec2 fragCoord) {
  float t = iTime;
  bool pressed = iMouse.z > 0.0;
  px = 1.0 / iResolution.y;

  // The pointer keeps its last position after release, so the view stays put.
  float yaw = t * 0.07 + iMouse.x / iResolution.x * 2.0 * PI;
  float feed = iMouse.y > 0.0 ? iMouse.y / iResolution.y : 0.25;
  float mass = mix(0.35, 1.6, feed);
  float hunger = smoothstep(1.15, 1.5, mass); // 0 calm, 1 ravenous

  // Screen tearing and shaking when it is hungry.
  vec2 uv = (fragCoord - 0.5 * iResolution.xy) / iResolution.y;
  float row = floor(uv.y * 60.0) + floor(t * 30.0);
  uv.x += (hash1(row) - 0.5) * 0.06 * hunger * step(0.8, hash1(row + 3.0));
  uv += (vec2(hash1(t), hash1(t + 1.7)) - 0.5) * 0.01 * hunger;

  float pitch = 0.16 + 0.08 * sin(t * 0.13);
  vec3 ro = 20.0 * vec3(cos(pitch) * sin(yaw), sin(pitch), cos(pitch) * cos(yaw));
  vec3 forward = normalize(-ro);
  vec3 right = normalize(cross(vec3(0.0, 1.0, 0.0), forward));
  vec3 up = cross(forward, right);
  // Hunger widens the lens until the world bulges.
  float zoom = 1.5 - 0.35 * hunger * (0.5 + 0.5 * sin(t * 9.0));
  vec3 rd = normalize(forward * zoom + right * uv.x + up * uv.y);

  vec3 col = trace(ro, rd, mass, t);
  col = 1.0 - exp(-col * 1.4);                             // tone map
  col = mix(col, col * vec3(1.6, 0.4, 0.5), hunger * 0.6); // red alert
  col *= 1.0 - 0.9 * dot(uv, uv);                          // vignette
  col *= 0.94 + 0.06 * sin(fragCoord.y * PI);              // scanlines

  // Labels.
  bool flicker = hunger > 0.5 && hash1(floor(t * 12.0)) < hunger * 0.8;
  float title = flicker
    ? text(uv, vec2(-0.84, 0.36), 0.09, int[16](73, 84, 32, 72, 85, 78, 71, 69, 82, 83, 0, 0, 0, 0, 0, 0))
    : text(uv, vec2(-0.84, 0.36), 0.09, int[16](69, 86, 69, 78, 84, 32, 72, 79, 82, 73, 90, 79, 78, 0, 0, 0));
  float label = max(title, text(uv, vec2(-0.84, 0.29), 0.05, int[16](109, 97, 115, 115, 32, 61, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)));
  label = max(label, number(uv, vec2(-0.84 + 7.0 * ADVANCE * 0.05, 0.29), 0.05, mass));
  vec3 ink = flicker ? vec3(1.0, 0.2, 0.2) : vec3(0.92, 0.94, 1.0);
  col = mix(col, ink, label);
  if (!pressed) {
    float blink = 0.5 + 0.5 * sin(t * 4.0);
    float hint = text(uv, vec2(-0.84, -0.46), 0.045, int[16](100, 114, 97, 103, 32, 116, 111, 32, 112, 108, 97, 121, 0, 0, 0, 0));
    col = mix(col, vec3(1.0, 0.6, 0.3), hint * blink);
  }

  fragColor = vec4(pow(col, vec3(0.85)), 1.0);
}
