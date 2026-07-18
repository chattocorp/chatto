type Glyph = {
  pattern: string[];
  emojis: string[];
};

export type WordmarkParticle = {
  id: string;
  emoji: string;
  glyph: number;
  row: number;
  layer: number;
  x: number;
  y: number;
  z: number;
  size: number;
  opacity: number;
  sparkles: boolean;
  sparkleDelay: number;
  sparkleDuration: number;
  burstDistance: number;
  fallbackAngle: number;
  burstRotation: number;
};

export type ProjectedParticle = {
  x: number;
  y: number;
  depth: number;
  scale: number;
};

export type ProjectionRotation = {
  cosX: number;
  sinX: number;
  cosY: number;
  sinY: number;
};

export type ExplosionFrame = {
  offset: number;
  rotation: number;
  scaleDelta: number;
  opacity: number;
};

export type CursorGravity = {
  pull: number;
};

export type ConstructionFrame = {
  opacity: number;
  scale: number;
  glow: number;
};

export type ConstructionLaserFrame = {
  progress: number;
  opacity: number;
};

export type ImpactLaserFrame = {
  headProgress: number;
  tailProgress: number;
  opacity: number;
};

export type SmokeFrame = {
  distanceProgress: number;
  opacity: number;
  scale: number;
};

export type LaserJitter = {
  progressOffset: number;
  x: number;
  y: number;
  intensity: number;
};

export type StarFieldParticle = {
  emoji: '✨' | '⭐' | '🌟';
  x: number;
  y: number;
  depth: number;
  size: number;
  opacity: number;
  driftX: number;
  driftY: number;
  twinklePhase: number;
  twinkleSpeed: number;
};

/** A small least-recently-used cache for generated rendering resources. */
export class BoundedLruCache<T> {
  readonly #entries = new Map<string, T>();
  readonly #maximumEntries: number;

  constructor(maximumEntries: number) {
    this.#maximumEntries = Math.max(1, Math.floor(maximumEntries));
  }

  get size(): number {
    return this.#entries.size;
  }

  get(key: string): T | undefined {
    const value = this.#entries.get(key);
    if (value === undefined) return undefined;
    this.#entries.delete(key);
    this.#entries.set(key, value);
    return value;
  }

  set(key: string, value: T): void {
    this.#entries.delete(key);
    this.#entries.set(key, value);
    if (this.#entries.size <= this.#maximumEntries) return;
    const oldestKey = this.#entries.keys().next().value;
    if (oldestKey !== undefined) this.#entries.delete(oldestKey);
  }

  clear(): void {
    this.#entries.clear();
  }
}

const glyphs: Glyph[] = [
  {
    pattern: ['011110', '110011', '110000', '110000', '110000', '110011', '011110'],
    emojis: ['💬', '🌊', '🫧', '💙']
  },
  {
    pattern: ['110011', '110011', '110011', '111111', '110011', '110011', '110011'],
    emojis: ['🌿', '🍀', '🐢', '🦜']
  },
  {
    pattern: ['011110', '110011', '110011', '111111', '110011', '110011', '110011'],
    emojis: ['⭐', '🌞', '🌻', '🍋']
  },
  {
    pattern: ['011110', '111111', '001100', '001100', '001100', '001100', '001100'],
    emojis: ['🍊', '🔥', '🧡', '🍁']
  },
  {
    pattern: ['011110', '111111', '001100', '001100', '001100', '001100', '001100'],
    emojis: ['🍇', '🔮', '💜', '🪁']
  },
  {
    pattern: ['011110', '110011', '110011', '110011', '110011', '110011', '011110'],
    emojis: ['🍒', '🍓', '❤️', '🌹']
  }
];

const layerDepths = [-48, -32, -16, 0];
const totalColumns = glyphs.reduce((total, glyph) => total + glyph.pattern[0].length, 0) + 5;
export const EXPLOSION_DURATION = 3000;
export const EXPLOSION_REBUILD_START = 0.48;
export const EXPLOSION_PARTICLE_FORCE_THRESHOLD = 0.08;
export const IMPACT_LASER_DURATION = 190;
export const SMOKE_DURATION = 900;
export const CONSTRUCTION_DURATION = 1650;
export const CANVAS_PIXEL_RATIO_LIMIT = 1.5;
export const LASER_COOLDOWN = 1500;
export const MAX_LASER_GUNS = 8;
export const GAME_UI_REVEAL_SHOTS = 3;

const CONSTRUCTION_FIRST_ROW_DELAY = 80;
const CONSTRUCTION_ROW_INTERVAL = 140;
const CONSTRUCTION_SWEEP_DURATION = 460;
const CONSTRUCTION_LASER_FADE_DURATION = 140;
const CONSTRUCTION_PARTICLE_SETTLE_DURATION = 160;

/** Cost of the next gun, with each additional gun becoming substantially dearer. */
export function laserGunCost(currentGunCount: number): number {
  const owned = Math.max(1, Math.min(MAX_LASER_GUNS, Math.floor(currentGunCount)));
  return Math.round(48 * Math.pow(1.9, owned - 1));
}

/** Cost of increasing the shared power level for every owned laser gun. */
export function laserPowerUpgradeCost(currentPower: number): number {
  const power = Math.max(1, Math.floor(currentPower));
  return Math.round(16 * Math.pow(1.55, power - 1));
}

/** Blast-radius scale for the current shared laser power level. */
export function laserPowerRadiusScale(power: number): number {
  return Math.min(0.16, 0.035 + (Math.max(1, Math.floor(power)) - 1) * 0.008);
}

/** Relative smoke size and density for a shot at the given laser power. */
export function laserPowerSmokeScale(power: number): number {
  return Math.min(1.5, 0.42 + (Math.max(1, Math.floor(power)) - 1) * 0.12);
}

export function nextReadyLaserIndex(readyAt: number[], now: number): number {
  return readyAt.findIndex((timestamp) => timestamp <= now);
}

export function laserCooldownProgress(now: number, readyAt: number): number {
  return Math.max(0, Math.min(1, 1 - (readyAt - now) / LASER_COOLDOWN));
}

function deterministicUnit(seed: number): number {
  const value = Math.sin(seed * 12.9898) * 43758.5453;
  return value - Math.floor(value);
}

/** Build a deterministic, depth-sorted emoji dust field for the canvas backdrop. */
export function createStarFieldParticles(count = 96): StarFieldParticle[] {
  return Array.from({ length: Math.max(0, Math.floor(count)) }, (_, index) => {
    const depth = 0.12 + deterministicUnit(index * 7 + 1) * 0.88;
    const emoji: StarFieldParticle['emoji'] =
      index % 13 === 0 ? '🌟' : index % 5 === 0 ? '⭐' : '✨';
    return {
      emoji,
      x: 0.025 + deterministicUnit(index * 11 + 2) * 0.95,
      y: 0.04 + deterministicUnit(index * 17 + 3) * 0.92,
      depth,
      size: 0.45 + depth * 0.72,
      opacity: 0.08 + depth * 0.34,
      driftX: (deterministicUnit(index * 19 + 4) - 0.5) * (0.002 + depth * 0.004),
      driftY: (deterministicUnit(index * 23 + 5) - 0.5) * (0.001 + depth * 0.002),
      twinklePhase: deterministicUnit(index * 29 + 6) * Math.PI * 2,
      twinkleSpeed: 0.45 + deterministicUnit(index * 31 + 7) * 1.25
    };
  }).sort((left, right) => left.depth - right.depth);
}

export function createWordmarkParticles(): WordmarkParticle[] {
  return glyphs.flatMap((glyph, glyphIndex) => {
    const columnOffset = glyphs
      .slice(0, glyphIndex)
      .reduce((total, previousGlyph) => total + previousGlyph.pattern[0].length + 1, 0);

    return glyph.pattern.flatMap((row, rowIndex) =>
      [...row].flatMap((cell, columnIndex) => {
        if (cell !== '1') return [];

        return layerDepths.map((depth, layerIndex) => {
          const variation = ((glyphIndex * 11 + rowIndex * 5 + columnIndex * 3) % 5) * 0.06;
          const emojiIndex =
            (glyphIndex + rowIndex + columnIndex + layerIndex) % glyph.emojis.length;
          const x = 0.04 + ((columnOffset + columnIndex) / (totalColumns - 1)) * 0.92;
          const y = 0.12 + (rowIndex / 6) * 0.76;
          const offsetX = x - 0.5;
          const offsetY = (y - 0.5) * 1.4;
          const vectorLength = Math.hypot(offsetX, offsetY) || 1;
          const sparkleSeed = glyphIndex * 97 + rowIndex * 31 + columnIndex * 17 + layerIndex * 13;
          const frontFacing = layerIndex === layerDepths.length - 1;

          return {
            id: `${glyphIndex}-${rowIndex}-${columnIndex}-${layerIndex}`,
            emoji: glyph.emojis[emojiIndex],
            glyph: glyphIndex,
            row: rowIndex,
            layer: layerIndex,
            x,
            y,
            z: depth,
            size: 0.62 + layerIndex * 0.1 + variation,
            opacity: 0.35 + layerIndex * 0.21,
            sparkles: frontFacing && sparkleSeed % 6 === 0,
            sparkleDelay: -((sparkleSeed * 37) % 8000),
            sparkleDuration: (7 + (sparkleSeed % 7)) * 1000,
            burstDistance:
              180 + ((glyphIndex * 13 + rowIndex * 7 + columnIndex * 5 + layerIndex) % 7) * 20,
            fallbackAngle: Math.atan2(offsetY / vectorLength, offsetX / vectorLength),
            burstRotation: -80 + ((rowIndex * 7 + columnIndex * 3 + layerIndex) % 9) * 20
          };
        });
      })
    );
  });
}

export function projectParticle(
  particle: WordmarkParticle,
  width: number,
  height: number,
  rotateX: number,
  rotateY: number
): ProjectedParticle {
  const radiansX = (rotateX * Math.PI) / 180;
  const radiansY = (rotateY * Math.PI) / 180;
  return projectParticleWithRotation(particle, width, height, {
    cosX: Math.cos(radiansX),
    sinX: Math.sin(radiansX),
    cosY: Math.cos(radiansY),
    sinY: Math.sin(radiansY)
  });
}

export function createProjectionRotation(rotateX: number, rotateY: number): ProjectionRotation {
  const radiansX = (rotateX * Math.PI) / 180;
  const radiansY = (rotateY * Math.PI) / 180;
  return {
    cosX: Math.cos(radiansX),
    sinX: Math.sin(radiansX),
    cosY: Math.cos(radiansY),
    sinY: Math.sin(radiansY)
  };
}

export function projectParticleWithRotation(
  particle: WordmarkParticle,
  width: number,
  height: number,
  rotation: ProjectionRotation
): ProjectedParticle {
  const sceneX = (particle.x - 0.5) * width;
  const sceneY = (particle.y - 0.5) * height;
  const sceneZ = particle.z * (width / 672);
  const rotatedX = sceneX * rotation.cosY + sceneZ * rotation.sinY;
  const yawedZ = -sceneX * rotation.sinY + sceneZ * rotation.cosY;
  const rotatedY = sceneY * rotation.cosX - yawedZ * rotation.sinX;
  const rotatedZ = sceneY * rotation.sinX + yawedZ * rotation.cosX;
  const perspective = 700 * (width / 672);
  const scale = perspective / Math.max(1, perspective - rotatedZ);

  return {
    x: width / 2 + rotatedX * scale,
    y: height / 2 + rotatedY * scale,
    depth: rotatedZ,
    scale
  };
}

export function radialForce(distance: number, radius: number): number {
  if (radius <= 0) return 0;
  const proximity = Math.max(0, 1 - distance / radius);
  return proximity * proximity;
}

export function ballisticDisplacement(
  initialVelocity: number,
  acceleration: number,
  time: number
): number {
  const elapsed = Math.max(0, time);
  return initialVelocity * elapsed + 0.5 * acceleration * elapsed * elapsed;
}

export function rayExitDistance(
  x: number,
  y: number,
  directionX: number,
  directionY: number,
  width: number,
  height: number
): number {
  const horizontalDistance =
    directionX > 0
      ? (width - x) / directionX
      : directionX < 0
        ? -x / directionX
        : Number.POSITIVE_INFINITY;
  const verticalDistance =
    directionY > 0
      ? (height - y) / directionY
      : directionY < 0
        ? -y / directionY
        : Number.POSITIVE_INFINITY;

  return Math.max(0, Math.min(horizontalDistance, verticalDistance));
}

export function exponentialSample(unitRandom: number, rate = 1): number {
  const bounded = Math.max(0, Math.min(1 - Number.EPSILON, unitRandom));
  if (bounded === 0) return 0;
  return -Math.log(1 - bounded) / Math.max(Number.EPSILON, rate);
}

export function cursorGravity(distance: number, radius: number, enabled = true): CursorGravity {
  if (!enabled) return { pull: 0 };
  const force = radialForce(distance, radius);
  if (force === 0 || distance <= 0) return { pull: 0 };
  return { pull: 8 * force * Math.min(1, distance / 18) };
}

function lerp(start: number, end: number, progress: number): number {
  return start + (end - start) * Math.max(0, Math.min(1, progress));
}

function easeOutExpo(progress: number): number {
  return progress >= 1 ? 1 : 1 - Math.pow(2, -10 * progress);
}

function easeInOutCubic(progress: number): number {
  return progress < 0.5
    ? 4 * progress * progress * progress
    : 1 - Math.pow(-2 * progress + 2, 3) / 2;
}

export function laserJitter(progress: number, seed: number): LaserJitter {
  const bounded = Math.max(0, Math.min(1, progress));
  if (bounded === 0 || bounded === 1) {
    return {
      progressOffset: 0,
      x: 0,
      y: 0,
      intensity: 0.78 + Math.sin(bounded * 61 + seed) * 0.22
    };
  }
  const envelope = Math.sin(bounded * Math.PI);
  const heldProgress = Math.floor(bounded * 24) / 24;
  return {
    progressOffset:
      ((heldProgress - bounded) * 0.38 +
        Math.sin(bounded * 37 + seed * 1.31) * 0.014 +
        Math.sin(bounded * 97 + seed * 0.47) * 0.007) *
      envelope,
    x:
      (Math.sin(bounded * 47 + seed * 1.7) * 5.5 + Math.sin(bounded * 113 + seed * 0.73) * 2.8) *
      envelope,
    y: Math.sin(bounded * 79 + seed * 2.3) * 3.4 * envelope,
    intensity: 0.78 + Math.sin(bounded * 61 + seed) * 0.22
  };
}

function constructionRowStart(row: number): number {
  return CONSTRUCTION_FIRST_ROW_DELAY + (6 - row) * CONSTRUCTION_ROW_INTERVAL;
}

export function constructionFrame(
  elapsed: number,
  particle: Pick<WordmarkParticle, 'row' | 'layer' | 'x'>
): ConstructionFrame {
  if (elapsed >= CONSTRUCTION_DURATION) return { opacity: 1, scale: 1, glow: 0 };
  const arrival =
    constructionRowStart(particle.row) +
    particle.x * CONSTRUCTION_SWEEP_DURATION +
    particle.layer * 18;
  const progress = Math.max(
    0,
    Math.min(1, (elapsed - arrival) / CONSTRUCTION_PARTICLE_SETTLE_DURATION)
  );
  if (progress === 0) return { opacity: 0, scale: 0.18, glow: 0 };

  const eased = easeOutExpo(progress);
  return {
    opacity: eased,
    scale: lerp(0.18, 1, eased),
    glow: 1 - progress
  };
}

export function canvasPixelRatio(devicePixelRatio: number): number {
  return Math.max(1, Math.min(CANVAS_PIXEL_RATIO_LIMIT, devicePixelRatio));
}

export function quantizeSpriteFontSize(fontSize: number): number {
  return Math.max(0.5, Math.round(fontSize * 2) / 2);
}

export function glyphFloatOffset(elapsed: number, glyph: number, reducedMotion = false): number {
  if (reducedMotion || elapsed <= 0) return 0;
  const entrance = easeInOutCubic(Math.min(1, elapsed / 600));
  const phase = (elapsed / 4200) * Math.PI * 2 + glyph * 0.82;
  return Math.sin(phase) * 8 * entrance;
}

export function explosionParticleOpacity(force: number, phaseOpacity: number): number {
  return force >= EXPLOSION_PARTICLE_FORCE_THRESHOLD ? phaseOpacity : 1;
}

export function constructionLaserFrame(
  elapsed: number,
  row: number
): ConstructionLaserFrame | null {
  const rowStart = constructionRowStart(row);
  const rowElapsed = elapsed - rowStart;
  const totalDuration = CONSTRUCTION_SWEEP_DURATION + CONSTRUCTION_LASER_FADE_DURATION;
  if (rowElapsed < 0 || rowElapsed >= totalDuration) return null;

  return {
    progress: Math.min(1, rowElapsed / CONSTRUCTION_SWEEP_DURATION),
    opacity:
      rowElapsed <= CONSTRUCTION_SWEEP_DURATION
        ? 1
        : 1 - (rowElapsed - CONSTRUCTION_SWEEP_DURATION) / CONSTRUCTION_LASER_FADE_DURATION
  };
}

export function impactLaserFrame(elapsed: number): ImpactLaserFrame | null {
  if (elapsed < 0 || elapsed >= IMPACT_LASER_DURATION) return null;
  const progress = elapsed / IMPACT_LASER_DURATION;
  const headProgress = progress * progress;

  return {
    headProgress,
    tailProgress: Math.max(0, headProgress - 0.24),
    opacity: Math.min(1, progress / 0.12)
  };
}

export function smokeFrame(elapsed: number, delay: number): SmokeFrame | null {
  const localElapsed = elapsed - delay;
  if (localElapsed < 0 || localElapsed >= SMOKE_DURATION) return null;
  const progress = localElapsed / SMOKE_DURATION;
  const entrance = Math.min(1, progress / 0.12);

  return {
    distanceProgress: easeOutExpo(progress),
    opacity: entrance * Math.pow(1 - progress, 1.35),
    scale: lerp(0.35, 1.1, easeOutExpo(progress))
  };
}

export function rebuildParticleFrame(
  progress: number,
  bottomToTop: number,
  leftToRight: number
): ConstructionFrame {
  if (progress >= 1) return { opacity: 1, scale: 1, glow: 0 };
  const arrival = rebuildParticleArrival(bottomToTop, leftToRight) + 0.025;
  const localProgress = Math.max(0, Math.min(1, (progress - arrival) / 0.12));
  if (localProgress === 0) return { opacity: 0, scale: 0.18, glow: 0 };
  const eased = easeOutExpo(localProgress);

  return {
    opacity: eased,
    scale: lerp(0.18, 1, eased),
    glow: 1 - localProgress
  };
}

function rebuildParticleArrival(bottomToTop: number, leftToRight: number): number {
  return (
    EXPLOSION_REBUILD_START +
    Math.max(0, Math.min(1, bottomToTop)) * 0.2 +
    Math.max(0, Math.min(1, leftToRight)) * 0.16
  );
}

export function rebuildStitchFrame(
  progress: number,
  bottomToTop: number,
  leftToRight: number
): ConstructionLaserFrame | null {
  const elapsed = progress - (rebuildParticleArrival(bottomToTop, leftToRight) - 0.065);
  const duration = 0.09;
  if (elapsed < 0 || elapsed >= duration) return null;
  const localProgress = elapsed / duration;

  return {
    progress: localProgress,
    opacity: localProgress < 0.72 ? 1 : 1 - (localProgress - 0.72) / 0.28
  };
}

export function explosionFrame(progress: number): ExplosionFrame {
  if (progress <= 0 || progress >= 1) {
    return { offset: 0, rotation: 0, scaleDelta: 0, opacity: 1 };
  }
  if (progress < 0.42) {
    const flightTime = progress / 0.42;
    const fade = easeInOutCubic(Math.max(0, Math.min(1, (flightTime - 0.72) / 0.28)));
    return {
      offset: flightTime,
      rotation: flightTime,
      scaleDelta: lerp(0, -0.18, flightTime),
      opacity: 1 - fade
    };
  }
  if (progress < EXPLOSION_REBUILD_START) {
    return { offset: 1, rotation: 1, scaleDelta: -0.18, opacity: 0 };
  }

  return { offset: 0, rotation: 0, scaleDelta: 0, opacity: 0 };
}

export function sparkleStrength(
  now: number,
  delay: number,
  duration: number,
  enabled: boolean
): number {
  if (!enabled || duration <= 0) return 0;
  const cycle = (((now - delay) % duration) + duration) % duration;
  const progress = cycle / duration;
  if (progress < 0.61 || progress >= 0.88) return 0;
  if (progress < 0.64) return (progress - 0.61) / 0.03;
  if (progress <= 0.68) return 1;
  return 1 - (progress - 0.68) / 0.2;
}
