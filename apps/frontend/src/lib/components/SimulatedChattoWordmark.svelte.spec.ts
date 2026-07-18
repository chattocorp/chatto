import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import SimulatedChattoWordmark from './SimulatedChattoWordmark.svelte';
import {
  ballisticDisplacement,
  BoundedLruCache,
  canvasPixelRatio,
  CONSTRUCTION_DURATION,
  constructionFrame,
  constructionLaserFrame,
  createProjectionRotation,
  createStarFieldParticles,
  createWordmarkParticles,
  cursorGravity,
  EXPLOSION_DURATION,
  EXPLOSION_PARTICLE_FORCE_THRESHOLD,
  exponentialSample,
  explosionFrame,
  explosionParticleOpacity,
  glyphFloatOffset,
  IMPACT_LASER_DURATION,
  impactLaserFrame,
  laserJitter,
  projectParticle,
  projectParticleWithRotation,
  quantizeSpriteFontSize,
  radialForce,
  rebuildParticleFrame,
  rebuildStitchFrame,
  rayExitDistance,
  smokeFrame,
  sparkleStrength
} from './simulatedChattoWordmark';

describe('SimulatedChattoWordmark', () => {
  it('renders the particle wordmark in one accessible canvas control', async () => {
    const { container } = render(SimulatedChattoWordmark);
    const wordmark = q(container, 'button[aria-label="Chatto"]');
    const canvas = q(container, 'canvas[aria-hidden="true"]') as HTMLCanvasElement;

    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(wordmark).toBeTruthy();
    expect(canvas.width).toBeGreaterThan(1);
    expect(wordmark?.classList.contains('rounded-lg')).toBe(true);
    expect(canvas.classList.contains('rounded-lg')).toBe(true);
    expect(Array.from(canvas.getContext('2d')!.getImageData(1, 1, 1, 1).data.slice(0, 3))).toEqual([
      5, 7, 12
    ]);
    expect(container.querySelectorAll('.emoji-point')).toHaveLength(0);
  });

  it('builds four depth layers for the rounded glyphs', () => {
    const particles = createWordmarkParticles();

    expect(particles).toHaveLength(600);
    expect(particles.filter((particle) => particle.z === 0)).toHaveLength(150);
    expect(particles.filter((particle) => particle.sparkles).length).toBeGreaterThan(15);
    expect(new Set(particles.map((particle) => particle.glyph))).toEqual(
      new Set([0, 1, 2, 3, 4, 5])
    );
    expect(Math.min(...particles.map((particle) => particle.burstDistance))).toBe(180);
    expect(Math.max(...particles.map((particle) => particle.burstDistance))).toBe(300);
  });

  it('builds deterministic depth-sorted emoji space dust', () => {
    const stars = createStarFieldParticles();

    expect(stars).toHaveLength(96);
    expect(stars).toEqual(createStarFieldParticles());
    expect(stars.every((star) => star.x >= 0 && star.x <= 1)).toBe(true);
    expect(stars.every((star) => star.y >= 0 && star.y <= 1)).toBe(true);
    expect(stars.every((star, index) => index === 0 || stars[index - 1].depth <= star.depth)).toBe(
      true
    );
    expect(new Set(stars.map((star) => star.emoji))).toEqual(new Set(['✨', '⭐', '🌟']));
  });

  it('floats whole characters in a staggered wave during construction', () => {
    const elapsed = 500;

    expect(glyphFloatOffset(0, 0)).toBe(0);
    expect(glyphFloatOffset(elapsed, 0)).not.toBe(0);
    expect(glyphFloatOffset(elapsed, 0)).not.toBeCloseTo(glyphFloatOffset(elapsed, 1));
    expect(glyphFloatOffset(elapsed, 0, true)).toBe(0);
  });

  it('constructs rows from bottom to top with horizontal laser sweeps', () => {
    const bottomLeft = constructionFrame(160, { row: 6, layer: 0, x: 0.04 });
    const bottomRight = constructionFrame(160, { row: 6, layer: 0, x: 0.96 });
    const topLeft = constructionFrame(160, { row: 0, layer: 0, x: 0.04 });

    expect(bottomLeft.opacity).toBeGreaterThan(0);
    expect(bottomRight.opacity).toBe(0);
    expect(topLeft.opacity).toBe(0);
    expect(constructionLaserFrame(100, 6)?.progress).toBeGreaterThan(0);
    expect(constructionLaserFrame(100, 0)).toBeNull();
    expect(constructionFrame(CONSTRUCTION_DURATION, { row: 0, layer: 3, x: 0.96 })).toEqual({
      opacity: 1,
      scale: 1,
      glow: 0
    });
  });

  it('caps the large drawing surface backing resolution', () => {
    expect(canvasPixelRatio(1)).toBe(1);
    expect(canvasPixelRatio(2)).toBe(1.5);
    expect(canvasPixelRatio(3)).toBe(1.5);
  });

  it('bounds generated sprite resources and quantizes their font sizes', () => {
    const cache = new BoundedLruCache<number>(2);
    cache.set('first', 1);
    cache.set('second', 2);
    expect(cache.get('first')).toBe(1);
    cache.set('third', 3);

    expect(cache.size).toBe(2);
    expect(cache.get('second')).toBeUndefined();
    expect(cache.get('first')).toBe(1);
    expect(cache.get('third')).toBe(3);
    expect(quantizeSpriteFontSize(20.24)).toBe(20);
    expect(quantizeSpriteFontSize(20.26)).toBe(20.5);
  });

  it('projects depth and Y rotation into screen coordinates', () => {
    const particle = createWordmarkParticles()[0];
    const flat = projectParticle(particle, 672, 134.4, 0, 0);
    const turned = projectParticle(particle, 672, 134.4, 0, 24);
    const cachedRotation = projectParticleWithRotation(
      particle,
      672,
      134.4,
      createProjectionRotation(0, 24)
    );

    expect(flat.x).not.toBe(turned.x);
    expect(flat.depth).not.toBe(turned.depth);
    expect(cachedRotation).toEqual(turned);
  });

  it('constrains explosion force to the local click radius', () => {
    expect(radialForce(0, 100)).toBe(1);
    expect(radialForce(50, 100)).toBe(0.25);
    expect(radialForce(100, 100)).toBe(0);
    expect(radialForce(500, 100)).toBe(0);
  });

  it('draws only nearby particles subtly toward the cursor', () => {
    const near = cursorGravity(5, 80);
    const far = cursorGravity(80, 80);

    expect(near.pull).toBeGreaterThan(0);
    expect(near.pull).toBeLessThanOrEqual(8);
    expect(far).toEqual({ pull: 0 });
  });

  it('scatters particles away before rebuilding them with lasers', () => {
    expect(EXPLOSION_DURATION).toBe(3000);
    expect(explosionFrame(0.1).offset).toBeCloseTo(0.1 / 0.42);
    expect(explosionFrame(0.42)).toEqual({
      offset: 1,
      rotation: 1,
      scaleDelta: -0.18,
      opacity: 0
    });
    expect(explosionFrame(0.58)).toEqual({
      offset: 0,
      rotation: 0,
      scaleDelta: 0,
      opacity: 0
    });
    expect(rebuildParticleFrame(0.4, 0, 0).opacity).toBe(0);
    expect(rebuildParticleFrame(0.7, 0, 0).opacity).toBe(1);
    expect(rebuildStitchFrame(0.44, 0, 0)?.progress).toBeGreaterThan(0);
    expect(rebuildParticleFrame(0.44, 0, 0).opacity).toBe(0);
    expect(rebuildStitchFrame(0.44, 1, 1)).toBeNull();
    expect(explosionFrame(1)).toEqual({
      offset: 0,
      rotation: 0,
      scaleDelta: 0,
      opacity: 1
    });
    expect(sparkleStrength(6600, 0, 10000, true)).toBe(1);
    expect(sparkleStrength(7800, 0, 10000, true)).toBeCloseTo(0.5);
  });

  it('keeps exploded particles hidden until their laser rebuild reaches them', () => {
    expect(explosionParticleOpacity(EXPLOSION_PARTICLE_FORCE_THRESHOLD, 0)).toBe(0);
    expect(explosionParticleOpacity(0.5, 0.25)).toBe(0.25);
    expect(explosionParticleOpacity(EXPLOSION_PARTICLE_FORCE_THRESHOLD / 2, 0)).toBe(1);
  });

  it('applies constant gravity only throughout an exploded particle flight', () => {
    const initialVelocity = -200;
    const gravity = 320;
    const step = 0.2;
    const positions = [0, 1, 2, 3].map((index) =>
      ballisticDisplacement(initialVelocity, gravity, index * step)
    );
    const velocities = positions
      .slice(1)
      .map((position, index) => (position - positions[index]) / step);

    expect(velocities[1] - velocities[0]).toBeCloseTo(gravity * step);
    expect(velocities[2] - velocities[1]).toBeCloseTo(gravity * step);
    expect(ballisticDisplacement(50, 0, 0.5)).toBe(25);
    expect(ballisticDisplacement(50, gravity, -1)).toBe(0);
  });

  it('sends strong explosion rays beyond the drawing surface with jittered rebuilds', () => {
    expect(rayExitDistance(50, 50, 1, 0, 100, 100)).toBe(50);
    expect(rayExitDistance(50, 50, 0, -1, 100, 100)).toBe(50);
    expect(laserJitter(0, 3)).toEqual({
      progressOffset: 0,
      x: 0,
      y: 0,
      intensity: expect.any(Number)
    });
    expect(Math.abs(laserJitter(0.5, 3).x)).toBeGreaterThan(0);
    expect(laserJitter(0.5, 3).progressOffset).not.toBe(0);
    expect(laserJitter(1, 3).x).toBeCloseTo(0);
    expect(exponentialSample(0)).toBe(0);
    expect(exponentialSample(0.9)).toBeGreaterThan(exponentialSample(0.5));
  });

  it('accelerates impact lasers before releasing fading cloud smoke', () => {
    const earlyLaser = impactLaserFrame(IMPACT_LASER_DURATION * 0.25);
    const lateLaser = impactLaserFrame(IMPACT_LASER_DURATION * 0.75);

    expect(earlyLaser?.headProgress).toBeCloseTo(0.0625);
    expect(lateLaser?.headProgress).toBeCloseTo(0.5625);
    expect(impactLaserFrame(IMPACT_LASER_DURATION)).toBeNull();
    expect(smokeFrame(0, 50)).toBeNull();
    expect(smokeFrame(200, 50)?.opacity).toBeGreaterThan(0.5);
    expect(smokeFrame(850, 0)?.opacity).toBeLessThan(0.1);
  });
});
