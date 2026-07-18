<script lang="ts">
  import { untrack } from 'svelte';
  import * as m from '$lib/i18n/messages';
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
    EXPLOSION_REBUILD_START,
    exponentialSample,
    explosionFrame,
    explosionParticleOpacity,
    GAME_UI_REVEAL_SHOTS,
    glyphFloatOffset,
    IMPACT_LASER_DURATION,
    impactLaserFrame,
    LASER_COOLDOWN,
    laserCooldownProgress,
    laserGunCost,
    laserJitter,
    laserPowerRadiusScale,
    laserPowerSmokeScale,
    laserPowerUpgradeCost,
    MAX_LASER_GUNS,
    nextReadyLaserIndex,
    projectParticleWithRotation,
    quantizeSpriteFontSize,
    radialForce,
    rebuildParticleFrame,
    rebuildStitchFrame,
    rayExitDistance,
    smokeFrame,
    sparkleStrength,
    type ProjectedParticle,
    type ProjectionRotation,
    type WordmarkParticle
  } from './simulatedChattoWordmark';

  type BurstVector = {
    x: number;
    y: number;
    force: number;
    rotation: number;
    gravity: number;
  };
  type ActiveBurst = {
    triggeredAt: number;
    startedAt: number;
    origin: { x: number; y: number };
    influenceRadius: number;
    vectors: BurstVector[];
    lasers: LaserBeam[];
    smoke: SmokeParticle[];
    smokeIntensity: number;
  };
  type LaserGunState = {
    id: number;
    power: number;
    readyAt: number;
  };
  type LaserBeam = {
    x: number;
    y: number;
  };
  type SmokeParticle = {
    angle: number;
    distance: number;
    delay: number;
    size: number;
  };
  type WordmarkBounds = {
    left: number;
    top: number;
    width: number;
    height: number;
  };
  type CanvasProjectionFrame = {
    wordmark: WordmarkBounds;
    rotation: ProjectionRotation;
    glyphOffsets: number[];
  };
  type EmojiSprite = {
    canvas: HTMLCanvasElement;
    width: number;
    height: number;
  };
  type RenderParticle = {
    index: number;
    particle: WordmarkParticle;
    position: ProjectedParticle;
  };

  const ACTIVE_FRAME_RATE = 60;
  const IDLE_FRAME_RATE = 30;
  const MAX_ACTIVE_BURSTS = 8;
  const MAX_EMOJI_SPRITES = 768;
  const FOREGROUND_STAR_DEPTH = 0.66;
  type StarFieldLayer = 'background' | 'foreground';

  type Props = {
    contained?: boolean;
    /** Optional starting state for isolated previews and component tests. */
    initialPoints?: number;
    initialLaserPowers?: number[];
  };

  let { contained = false, initialPoints = 0, initialLaserPowers = [1] }: Props = $props();
  const drawingSurfaceWidthScale = $derived(contained ? 1.25 : 1.7);
  const startingLaserPowers = untrack(() =>
    initialLaserPowers
      .filter((power) => Number.isFinite(power))
      .slice(0, MAX_LASER_GUNS)
      .map((power) => Math.max(1, Math.floor(power)))
  );

  const particles = createWordmarkParticles();
  const stars = createStarFieldParticles();
  const renderParticles: RenderParticle[] = particles.map((particle, index) => ({
    index,
    particle,
    position: { x: 0, y: 0, depth: particle.z, scale: 1 }
  }));
  const emojiSprites = new BoundedLruCache<EmojiSprite>(MAX_EMOJI_SPRITES);

  let canvasContext: CanvasRenderingContext2D | null = null;
  let canvasElement: HTMLCanvasElement | null = null;
  let canvasWidth = 0;
  let canvasHeight = 0;
  let animationFrame: number | undefined;
  let canvasInViewport = true;
  let lastDrawnAt = Number.NEGATIVE_INFINITY;
  let lastSortedRotateX = Number.NaN;
  let lastSortedRotateY = Number.NaN;
  let targetRotateX = 0;
  let targetRotateY = 0;
  let currentRotateX = 0;
  let currentRotateY = 0;
  let constructionStartedAt = Number.NEGATIVE_INFINITY;
  let activeBursts: ActiveBurst[] = [];
  let reducedMotion = false;
  let hoverCursor: { x: number; y: number } | null = null;
  let points = $state(
    untrack(() => (Number.isFinite(initialPoints) ? Math.max(0, Math.floor(initialPoints)) : 0))
  );
  let firstLaserShots = $state(0);
  let laserGuns = $state.raw<LaserGunState[]>(
    (startingLaserPowers.length > 0 ? startingLaserPowers : [1]).map((power, index) => ({
      id: index + 1,
      power,
      readyAt: 0
    }))
  );
  let hudNow = $state(0);

  const nextLaserCost = $derived(laserGunCost(laserGuns.length));
  const gameUiVisible = $derived(firstLaserShots >= GAME_UI_REVEAL_SHOTS);

  function setupCanvas(canvas: HTMLCanvasElement) {
    canvasElement = canvas;
    canvasContext = canvas.getContext('2d');
    constructionStartedAt = performance.now();
    hudNow = constructionStartedAt;
    const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
    reducedMotion = motionQuery.matches;

    function resizeCanvas() {
      const bounds = canvas.getBoundingClientRect();
      const pixelRatio = canvasPixelRatio(window.devicePixelRatio || 1);
      canvasWidth = bounds.width;
      canvasHeight = bounds.height;
      canvas.width = Math.max(1, Math.round(bounds.width * pixelRatio));
      canvas.height = Math.max(1, Math.round(bounds.height * pixelRatio));
      emojiSprites.clear();
      lastDrawnAt = Number.NEGATIVE_INFINITY;
      lastSortedRotateX = Number.NaN;
      lastSortedRotateY = Number.NaN;
      requestDraw();
    }

    function handleMotionPreference() {
      reducedMotion = motionQuery.matches;
      if (reducedMotion) {
        currentRotateX = targetRotateX;
        currentRotateY = targetRotateY;
      }
      requestDraw();
    }

    function handleVisibilityChange() {
      if (document.hidden) {
        if (animationFrame !== undefined) cancelAnimationFrame(animationFrame);
        animationFrame = undefined;
        return;
      }
      requestDraw();
    }

    const resizeObserver = new ResizeObserver(resizeCanvas);
    const intersectionObserver = new IntersectionObserver(([entry]) => {
      canvasInViewport = entry?.isIntersecting ?? true;
      if (canvasInViewport) {
        lastDrawnAt = Number.NEGATIVE_INFINITY;
        requestDraw();
      } else if (animationFrame !== undefined) {
        cancelAnimationFrame(animationFrame);
        animationFrame = undefined;
      }
    });
    resizeObserver.observe(canvas);
    intersectionObserver.observe(canvas.parentElement ?? canvas);
    motionQuery.addEventListener('change', handleMotionPreference);
    document.addEventListener('visibilitychange', handleVisibilityChange);
    resizeCanvas();

    return () => {
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
      motionQuery.removeEventListener('change', handleMotionPreference);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      if (animationFrame !== undefined) cancelAnimationFrame(animationFrame);
      animationFrame = undefined;
      canvasContext = null;
      canvasElement = null;
      emojiSprites.clear();
    };
  }

  function requestDraw() {
    if (animationFrame === undefined && !document.hidden && canvasInViewport) {
      animationFrame = requestAnimationFrame(draw);
    }
  }

  function getWordmarkBounds(): WordmarkBounds {
    const width = canvasWidth / drawingSurfaceWidthScale;
    const height = width / 5;

    return {
      left: (canvasWidth - width) / 2,
      top: (canvasHeight - height) / 2,
      width,
      height
    };
  }

  function createCanvasProjectionFrame(now: number): CanvasProjectionFrame {
    const elapsed = now - constructionStartedAt;
    return {
      wordmark: getWordmarkBounds(),
      rotation: createProjectionRotation(currentRotateX, currentRotateY),
      glyphOffsets: Array.from({ length: 6 }, (_, glyph) =>
        glyphFloatOffset(elapsed, glyph, reducedMotion)
      )
    };
  }

  function projectForCanvas(
    particle: WordmarkParticle,
    frame: CanvasProjectionFrame
  ): ProjectedParticle {
    const position = projectParticleWithRotation(
      particle,
      frame.wordmark.width,
      frame.wordmark.height,
      frame.rotation
    );

    return {
      ...position,
      x: position.x + frame.wordmark.left,
      y: position.y + frame.wordmark.top + frame.glyphOffsets[particle.glyph]
    };
  }

  function getEmojiSprite(emoji: string, fontSize: number, pixelRatio: number): EmojiSprite {
    const roundedFontSize = quantizeSpriteFontSize(fontSize);
    const key = `${emoji}:${roundedFontSize}:${pixelRatio}`;
    const cached = emojiSprites.get(key);
    if (cached) return cached;

    const logicalSize = Math.ceil(roundedFontSize * 2.2);
    const spriteCanvas = document.createElement('canvas');
    spriteCanvas.width = Math.max(1, Math.ceil(logicalSize * pixelRatio));
    spriteCanvas.height = Math.max(1, Math.ceil(logicalSize * pixelRatio));
    const context = spriteCanvas.getContext('2d');
    if (context) {
      context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
      context.textAlign = 'center';
      context.textBaseline = 'middle';
      context.font = `${roundedFontSize}px "Apple Color Emoji", "Segoe UI Emoji", sans-serif`;
      context.fillText(emoji, logicalSize / 2, logicalSize / 2);
    }

    const sprite = { canvas: spriteCanvas, width: logicalSize, height: logicalSize };
    emojiSprites.set(key, sprite);
    return sprite;
  }

  function drawConstructionLasers(context: CanvasRenderingContext2D, elapsed: number) {
    if (elapsed >= CONSTRUCTION_DURATION) return;
    const wordmark = getWordmarkBounds();

    for (let row = 6; row >= 0; row -= 1) {
      const laser = constructionLaserFrame(elapsed, row);
      if (!laser) continue;

      const jitter = laserJitter(laser.progress, row);
      const jitteredProgress = Math.max(0, Math.min(1, laser.progress + jitter.progressOffset));
      const headX = wordmark.left + wordmark.width * (0.04 + jitteredProgress * 0.92) + jitter.x;
      const tailX = Math.max(wordmark.left + wordmark.width * 0.025, headX - wordmark.width * 0.18);
      const y = wordmark.top + wordmark.height * (0.12 + (row / 6) * 0.76) + jitter.y;
      const trail = context.createLinearGradient(tailX, y, headX, y);
      trail.addColorStop(0, 'rgb(40 220 255 / 0)');
      trail.addColorStop(0.72, `rgb(40 220 255 / ${0.45 * laser.opacity})`);
      trail.addColorStop(1, `rgb(225 255 255 / ${laser.opacity})`);

      context.save();
      context.strokeStyle = trail;
      context.lineCap = 'round';
      context.lineWidth = 1.4 + jitter.intensity;
      context.shadowColor = `rgb(42 226 255 / ${laser.opacity})`;
      context.shadowBlur = 18;
      context.beginPath();
      context.moveTo(tailX, y);
      context.lineTo(headX, y);
      context.stroke();

      context.fillStyle = `rgb(240 255 255 / ${laser.opacity})`;
      context.shadowBlur = 24;
      context.fillRect(headX - 1, y - 7, 2, 14);
      context.restore();
    }
  }

  function drawImpactLasers(
    context: CanvasRenderingContext2D,
    activeBurst: ActiveBurst,
    now: number
  ) {
    const laser = impactLaserFrame(now - activeBurst.triggeredAt);
    if (!laser) return;

    for (const start of activeBurst.lasers) {
      const headX = start.x + (activeBurst.origin.x - start.x) * laser.headProgress;
      const headY = start.y + (activeBurst.origin.y - start.y) * laser.headProgress;
      const tailX = start.x + (activeBurst.origin.x - start.x) * laser.tailProgress;
      const tailY = start.y + (activeBurst.origin.y - start.y) * laser.tailProgress;
      const trail = context.createLinearGradient(tailX, tailY, headX, headY);
      trail.addColorStop(0, 'rgb(30 210 255 / 0)');
      trail.addColorStop(0.7, `rgb(34 225 255 / ${0.7 * laser.opacity})`);
      trail.addColorStop(1, `rgb(245 255 255 / ${laser.opacity})`);

      context.save();
      context.strokeStyle = trail;
      context.lineWidth = 2.5;
      context.lineCap = 'round';
      context.shadowColor = `rgb(35 225 255 / ${laser.opacity})`;
      context.shadowBlur = 20;
      context.beginPath();
      context.moveTo(tailX, tailY);
      context.lineTo(headX, headY);
      context.stroke();
      context.fillStyle = `rgb(245 255 255 / ${laser.opacity})`;
      context.beginPath();
      context.arc(headX, headY, 2.5, 0, Math.PI * 2);
      context.fill();
      context.restore();
    }
  }

  function drawBurstSmoke(
    context: CanvasRenderingContext2D,
    activeBurst: ActiveBurst,
    now: number,
    viewportFontSize: number,
    pixelRatio: number
  ) {
    const elapsed = now - activeBurst.startedAt;
    for (const smoke of activeBurst.smoke) {
      const frame = smokeFrame(elapsed, smoke.delay);
      if (!frame) continue;
      const distance = smoke.distance * frame.distanceProgress;
      const x = activeBurst.origin.x + Math.cos(smoke.angle) * distance;
      const y =
        activeBurst.origin.y + Math.sin(smoke.angle) * distance - 16 * frame.distanceProgress;
      const sprite = getEmojiSprite('☁️', viewportFontSize * smoke.size, pixelRatio);

      context.save();
      context.translate(x, y);
      context.scale(frame.scale, frame.scale);
      context.globalAlpha = frame.opacity * 0.72 * activeBurst.smokeIntensity;
      context.drawImage(
        sprite.canvas,
        -sprite.width / 2,
        -sprite.height / 2,
        sprite.width,
        sprite.height
      );
      context.restore();
    }
  }

  function drawRebuildStitches(
    context: CanvasRenderingContext2D,
    activeBurst: ActiveBurst,
    progress: number,
    projectionFrame: CanvasProjectionFrame
  ) {
    for (let index = 0; index < activeBurst.vectors.length; index += 1) {
      const vector = activeBurst.vectors[index];
      if (vector.force < 0.08 || particles[index].layer !== 3) continue;
      const target = projectForCanvas(particles[index], projectionFrame);
      const bottomToTop = Math.max(
        0,
        Math.min(
          1,
          (activeBurst.origin.y + activeBurst.influenceRadius - target.y) /
            (activeBurst.influenceRadius * 2)
        )
      );
      const leftToRight = Math.max(
        0,
        Math.min(
          1,
          (target.x - activeBurst.origin.x + activeBurst.influenceRadius) /
            (activeBurst.influenceRadius * 2)
        )
      );
      const laser = rebuildStitchFrame(progress, bottomToTop, leftToRight);
      if (!laser) continue;
      const jitter = laserJitter(laser.progress, index);
      const jitteredProgress = Math.max(0, Math.min(1, laser.progress + jitter.progressOffset));
      const headX = target.x - (1 - jitteredProgress) * 30 + jitter.x;
      const headY = target.y + jitter.y;
      const tailX = headX - 22 - jitter.x * 0.4;
      const trail = context.createLinearGradient(tailX, headY, headX, headY);
      trail.addColorStop(0, 'rgb(30 210 255 / 0)');
      trail.addColorStop(0.7, `rgb(35 225 255 / ${0.58 * laser.opacity})`);
      trail.addColorStop(1, `rgb(245 255 255 / ${laser.opacity})`);

      context.save();
      context.strokeStyle = trail;
      context.lineWidth = 1.2 + jitter.intensity;
      context.lineCap = 'round';
      context.shadowColor = `rgb(35 225 255 / ${laser.opacity})`;
      context.shadowBlur = 18;
      context.beginPath();
      context.moveTo(tailX, headY);
      context.lineTo(headX, headY);
      context.stroke();
      context.fillStyle = `rgb(245 255 255 / ${laser.opacity})`;
      context.fillRect(headX - 1, headY - 5, 2, 10);
      context.restore();
    }
  }

  function drawStarField(
    context: CanvasRenderingContext2D,
    now: number,
    pixelRatio: number,
    layer: StarFieldLayer
  ) {
    context.save();
    if (layer === 'background') {
      context.fillStyle = '#05070c';
      context.fillRect(0, 0, canvasWidth, canvasHeight);
    }

    const elapsed = reducedMotion ? 0 : now - constructionStartedAt;
    const baseFontSize = Math.min(13, Math.max(6, canvasWidth * 0.016));
    for (const star of stars) {
      const foreground = star.depth >= FOREGROUND_STAR_DEPTH;
      if ((layer === 'foreground') !== foreground) continue;
      const foregroundDepth = foreground
        ? (star.depth - FOREGROUND_STAR_DEPTH) / (1 - FOREGROUND_STAR_DEPTH)
        : 0;
      const parallaxMultiplier = foreground ? 1.45 + foregroundDepth * 0.55 : 1;
      const parallaxX = currentRotateY * star.depth * 0.72 * parallaxMultiplier;
      const parallaxY = -currentRotateX * star.depth * 0.56 * parallaxMultiplier;
      const x =
        (((star.x * canvasWidth + parallaxX + elapsed * star.driftX) % canvasWidth) + canvasWidth) %
        canvasWidth;
      const y =
        (((star.y * canvasHeight + parallaxY + elapsed * star.driftY) % canvasHeight) +
          canvasHeight) %
        canvasHeight;
      const twinkle = reducedMotion
        ? 0.72
        : 0.68 + Math.sin(elapsed * 0.001 * star.twinkleSpeed + star.twinklePhase) * 0.24;
      const sprite = getEmojiSprite(
        star.emoji,
        baseFontSize * star.size * (1 + foregroundDepth * 0.45),
        pixelRatio
      );

      context.globalAlpha = Math.min(0.72, star.opacity * twinkle * (foreground ? 1.35 : 1));
      context.drawImage(
        sprite.canvas,
        x - sprite.width / 2,
        y - sprite.height / 2,
        sprite.width,
        sprite.height
      );
    }
    context.restore();
  }

  function draw(now: number) {
    animationFrame = undefined;
    const context = canvasContext;
    if (!context || canvasWidth <= 0 || canvasHeight <= 0) return;
    activeBursts = activeBursts.filter(
      (activeBurst) => now - activeBurst.startedAt < EXPLOSION_DURATION
    );
    const constructionElapsed = reducedMotion ? CONSTRUCTION_DURATION : now - constructionStartedAt;
    const rotationSettling =
      Math.abs(targetRotateX - currentRotateX) > 0.02 ||
      Math.abs(targetRotateY - currentRotateY) > 0.02;
    const cooldownActive = laserGuns.some((laser) => laser.readyAt > now);
    if (cooldownActive && now - hudNow >= 50) hudNow = now;
    const activeMotion =
      constructionElapsed < CONSTRUCTION_DURATION ||
      activeBursts.length > 0 ||
      cooldownActive ||
      hoverCursor !== null ||
      rotationSettling;
    const frameInterval = 1000 / (activeMotion ? ACTIVE_FRAME_RATE : IDLE_FRAME_RATE);
    const elapsedSinceLastDraw = now - lastDrawnAt;
    if (elapsedSinceLastDraw < frameInterval - 1) {
      requestDraw();
      return;
    }
    lastDrawnAt = Number.isFinite(lastDrawnAt) ? now - (elapsedSinceLastDraw % frameInterval) : now;

    if (reducedMotion) {
      currentRotateX = targetRotateX;
      currentRotateY = targetRotateY;
    } else {
      currentRotateX += (targetRotateX - currentRotateX) * 0.12;
      currentRotateY += (targetRotateY - currentRotateY) * 0.12;
    }

    const pixelRatio = canvasPixelRatio(window.devicePixelRatio || 1);
    context.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
    context.clearRect(0, 0, canvasWidth, canvasHeight);

    const viewportFontSize = Math.min(20, Math.max(10.88, window.innerWidth * 0.02));
    const projectionFrame = createCanvasProjectionFrame(now);
    drawStarField(context, now, pixelRatio, 'background');
    drawConstructionLasers(context, constructionElapsed);
    const burstFrames = reducedMotion
      ? []
      : activeBursts.map((activeBurst) => ({
          activeBurst,
          progress: (now - activeBurst.startedAt) / EXPLOSION_DURATION,
          frame: explosionFrame((now - activeBurst.startedAt) / EXPLOSION_DURATION)
        }));
    if (!reducedMotion) {
      for (const activeBurst of activeBursts) {
        drawImpactLasers(context, activeBurst, now);
      }
      for (const { activeBurst, progress } of burstFrames) {
        if (progress <= 0 || progress >= 0.32) continue;
        const shockwaveProgress = progress / 0.32;
        const shockwaveRadius = activeBurst.influenceRadius * (0.12 + shockwaveProgress * 1.18);
        const shockwaveAlpha = (1 - shockwaveProgress) * 0.7;
        context.save();
        context.beginPath();
        context.arc(activeBurst.origin.x, activeBurst.origin.y, shockwaveRadius, 0, Math.PI * 2);
        context.strokeStyle = `rgb(255 255 255 / ${shockwaveAlpha})`;
        context.lineWidth = 1 + (1 - shockwaveProgress) * 4;
        context.shadowColor = `rgb(255 255 255 / ${shockwaveAlpha})`;
        context.shadowBlur = 12 * (1 - shockwaveProgress);
        context.stroke();
        context.restore();
      }
      for (const activeBurst of activeBursts) {
        drawBurstSmoke(context, activeBurst, now, viewportFontSize, pixelRatio);
      }
      for (const { activeBurst, progress } of burstFrames) {
        drawRebuildStitches(context, activeBurst, progress, projectionFrame);
      }
    }
    for (const entry of renderParticles) {
      entry.position = projectForCanvas(entry.particle, projectionFrame);
    }
    if (
      !Number.isFinite(lastSortedRotateX) ||
      Math.abs(currentRotateX - lastSortedRotateX) > 0.05 ||
      Math.abs(currentRotateY - lastSortedRotateY) > 0.05
    ) {
      renderParticles.sort((left, right) => left.position.depth - right.position.depth);
      lastSortedRotateX = currentRotateX;
      lastSortedRotateY = currentRotateY;
    }

    for (const { index, particle, position } of renderParticles) {
      const construction = constructionFrame(constructionElapsed, particle);
      if (construction.opacity <= 0) continue;
      let burstX = 0;
      let burstY = 0;
      let burstRotation = 0;
      let burstScaleDelta = 0;
      let burstOpacity = 1;
      let rebuildScale = 1;
      let rebuildGlow = 0;
      for (const { activeBurst, frame, progress } of burstFrames) {
        const vector = activeBurst.vectors[index];
        burstX += vector.x * frame.offset;
        burstY += ballisticDisplacement(vector.y, vector.gravity, frame.offset);
        burstRotation += vector.rotation * frame.rotation;
        burstScaleDelta += frame.scaleDelta * vector.force;

        if (progress >= EXPLOSION_REBUILD_START) {
          const bottomToTop = Math.max(
            0,
            Math.min(
              1,
              (activeBurst.origin.y + activeBurst.influenceRadius - position.y) /
                (activeBurst.influenceRadius * 2)
            )
          );
          const leftToRight = Math.max(
            0,
            Math.min(
              1,
              (position.x - activeBurst.origin.x + activeBurst.influenceRadius) /
                (activeBurst.influenceRadius * 2)
            )
          );
          const rebuild = rebuildParticleFrame(progress, bottomToTop, leftToRight);
          burstOpacity *= explosionParticleOpacity(vector.force, rebuild.opacity);
          rebuildScale *= 1 - vector.force * (1 - rebuild.scale);
          rebuildGlow = Math.max(rebuildGlow, rebuild.glow * vector.force);
        } else {
          burstOpacity *= explosionParticleOpacity(vector.force, frame.opacity);
        }
      }
      let hoverX = 0;
      let hoverY = 0;

      if (hoverCursor && !reducedMotion) {
        const directionX = hoverCursor.x - position.x;
        const directionY = hoverCursor.y - position.y;
        const distance = Math.hypot(directionX, directionY);
        if (distance > 0) {
          const gravity = cursorGravity(distance, projectionFrame.wordmark.width * 0.14);
          hoverX = (directionX / distance) * gravity.pull;
          hoverY = (directionY / distance) * gravity.pull;
        }
      }

      const x = position.x + burstX + hoverX;
      const y = position.y + burstY + hoverY;
      const scale =
        position.scale * construction.scale * rebuildScale * Math.max(0.15, 1 + burstScaleDelta);
      const sparkle = reducedMotion
        ? 0
        : sparkleStrength(now, particle.sparkleDelay, particle.sparkleDuration, particle.sparkles);
      const opacity = particle.opacity * construction.opacity * burstOpacity;
      if (opacity <= 0.002) continue;
      const constructionGlow = Math.max(construction.glow, rebuildGlow);
      const cullingRadius = viewportFontSize * particle.size * scale * 1.1 + constructionGlow * 20;
      if (
        x + cullingRadius < 0 ||
        x - cullingRadius > canvasWidth ||
        y + cullingRadius < 0 ||
        y - cullingRadius > canvasHeight
      ) {
        continue;
      }
      const sprite = getEmojiSprite(particle.emoji, viewportFontSize * particle.size, pixelRatio);

      context.save();
      context.translate(x, y);
      if (burstRotation !== 0) context.rotate((burstRotation * Math.PI) / 180);
      context.scale(scale, scale);
      context.globalAlpha = opacity;
      if (constructionGlow > 0) {
        context.shadowColor = `rgb(42 226 255 / ${constructionGlow})`;
        context.shadowBlur = 20 * constructionGlow;
      }
      if (sparkle > 0) {
        context.filter = `brightness(${1 - sparkle}) invert(${sparkle}) drop-shadow(0 0 ${7 * sparkle}px rgb(255 255 255 / ${0.95 * sparkle}))`;
      }
      context.drawImage(
        sprite.canvas,
        -sprite.width / 2,
        -sprite.height / 2,
        sprite.width,
        sprite.height
      );
      context.restore();
    }

    drawStarField(context, now, pixelRatio, 'foreground');

    if (!reducedMotion || cooldownActive) requestDraw();
  }

  function handlePointerMove(event: PointerEvent) {
    if (event.pointerType !== 'mouse' && event.pointerType !== 'pen') return;
    targetRotateX = (event.clientY / window.innerHeight - 0.5) * -28;
    targetRotateY = (event.clientX / window.innerWidth - 0.5) * 84;

    const bounds = canvasElement?.getBoundingClientRect();
    if (
      bounds &&
      event.clientX >= bounds.left &&
      event.clientX <= bounds.right &&
      event.clientY >= bounds.top &&
      event.clientY <= bounds.bottom
    ) {
      hoverCursor = { x: event.clientX - bounds.left, y: event.clientY - bounds.top };
    } else {
      hoverCursor = null;
    }
    requestDraw();
  }

  function triggerBurst(event: MouseEvent) {
    if (canvasWidth <= 0 || canvasHeight <= 0) return;

    const wordmark = event.currentTarget as HTMLButtonElement;
    const bounds = canvasElement?.getBoundingClientRect() ?? wordmark.getBoundingClientRect();
    const keyboardActivation = event.detail === 0;
    const originX = keyboardActivation ? canvasWidth / 2 : event.clientX - bounds.left;
    const originY = keyboardActivation ? canvasHeight / 2 : event.clientY - bounds.top;
    const triggeredAt = performance.now();
    const introShot = !gameUiVisible;
    const laserIndex = introShot
      ? (laserGuns[0]?.readyAt ?? Number.POSITIVE_INFINITY) <= triggeredAt
        ? 0
        : -1
      : nextReadyLaserIndex(
          laserGuns.map((laser) => laser.readyAt),
          triggeredAt
        );
    if (laserIndex < 0) return;
    const firingLaser = laserGuns[laserIndex];
    if (!firingLaser) return;
    const shotPower = firingLaser.power;
    laserGuns = laserGuns.map((laser, index) =>
      index === laserIndex ? { ...laser, readyAt: triggeredAt + LASER_COOLDOWN } : laser
    );
    if (introShot) firstLaserShots += 1;
    hudNow = triggeredAt;
    const projectionFrame = createCanvasProjectionFrame(triggeredAt);
    const influenceRadius = projectionFrame.wordmark.width * laserPowerRadiusScale(shotPower);
    const clamp = (value: number, minimum: number, maximum: number) =>
      Math.max(minimum, Math.min(maximum, value));
    const laserPositions: LaserBeam[] = [
      { x: 0, y: clamp(canvasHeight * 0.34, 20, canvasHeight - 20) },
      { x: canvasWidth, y: clamp(canvasHeight * 0.66, 20, canvasHeight - 20) },
      { x: clamp(canvasWidth * 0.3, 20, canvasWidth - 20), y: 0 },
      { x: clamp(canvasWidth * 0.7, 20, canvasWidth - 20), y: canvasHeight },
      { x: 0, y: clamp(canvasHeight * 0.76, 20, canvasHeight - 20) },
      { x: canvasWidth, y: clamp(canvasHeight * 0.24, 20, canvasHeight - 20) },
      { x: clamp(canvasWidth * 0.72, 20, canvasWidth - 20), y: 0 },
      { x: clamp(canvasWidth * 0.28, 20, canvasWidth - 20), y: canvasHeight }
    ];
    const lasers = [laserPositions[laserIndex]];
    const smokeScale = laserPowerSmokeScale(shotPower);
    const smokeCount = Math.round(5 + smokeScale * 9);
    const smoke: SmokeParticle[] = Array.from({ length: smokeCount }, (_, index) => {
      const angleDirection = Math.random() < 0.5 ? -1 : 1;
      return {
        angle:
          (index / smokeCount) * Math.PI * 2 +
          (originX + originY) * 0.001 +
          angleDirection * Math.min(0.5, exponentialSample(Math.random()) * 0.12),
        distance:
          (28 + (index % 4) * 8) * (1 + Math.min(0.65, exponentialSample(Math.random()) * 0.18)),
        delay: Math.min(180, exponentialSample(Math.random()) * 52),
        size:
          (2.2 + (index % 3) * 0.4) *
          smokeScale *
          (1 + Math.min(0.9, exponentialSample(Math.random()) * 0.22))
      };
    });
    const vectors = particles.map((particle) => {
      const position = projectForCanvas(particle, projectionFrame);
      let vectorX = position.x - originX;
      let vectorY = position.y - originY;
      let distance = Math.hypot(vectorX, vectorY);

      if (distance < 1) {
        vectorX = Math.cos(particle.fallbackAngle);
        vectorY = Math.sin(particle.fallbackAngle);
        distance = 1;
      }

      const radialStrength = radialForce(distance, influenceRadius);
      const rawForce = Math.min(1, radialStrength / 0.04);
      const force = rawForce >= EXPLOSION_PARTICLE_FORCE_THRESHOLD ? rawForce : 0;
      const angularDirection = Math.random() < 0.5 ? -1 : 1;
      const angularSpread = Math.min(0.8, exponentialSample(Math.random()) * 0.2);
      const trajectoryAngle = Math.atan2(vectorY, vectorX) + angularDirection * angularSpread;
      const directionX = Math.cos(trajectoryAngle);
      const directionY = Math.sin(trajectoryAngle);
      const exitDistance = rayExitDistance(
        position.x,
        position.y,
        directionX,
        directionY,
        canvasWidth,
        canvasHeight
      );
      const travelDistance =
        Math.max(particle.burstDistance, exitDistance + 96 + particle.burstDistance * 0.35) *
        force *
        (1 + Math.min(0.75, exponentialSample(Math.random()) * 0.18));
      const gravity = force * (220 + Math.min(600, exponentialSample(Math.random()) * 180));
      return {
        x: directionX * travelDistance,
        y: directionY * travelDistance - 0.5 * gravity,
        force,
        rotation: particle.burstRotation * force,
        gravity
      };
    });

    points += vectors.filter((vector) => vector.force >= EXPLOSION_PARTICLE_FORCE_THRESHOLD).length;

    if (reducedMotion) {
      requestDraw();
      return;
    }

    activeBursts = [
      ...activeBursts.slice(-(MAX_ACTIVE_BURSTS - 1)),
      {
        triggeredAt,
        startedAt: triggeredAt + IMPACT_LASER_DURATION,
        origin: { x: originX, y: originY },
        influenceRadius,
        vectors,
        lasers,
        smoke,
        smokeIntensity: Math.min(1, 0.35 + smokeScale * 0.5)
      }
    ];
    requestDraw();
  }

  function upgradeLaserPower(laserIndex: number) {
    const laser = laserGuns[laserIndex];
    if (!laser) return;
    const upgradeCost = laserPowerUpgradeCost(laser.power);
    if (points < upgradeCost) return;
    points -= upgradeCost;
    laserGuns = laserGuns.map((laser, index) =>
      index === laserIndex ? { ...laser, power: laser.power + 1 } : laser
    );
  }

  function buyLaserGun() {
    if (laserGuns.length >= MAX_LASER_GUNS || points < nextLaserCost) return;
    points -= nextLaserCost;
    const newLaserIndex = laserGuns.length;
    laserGuns = [...laserGuns, { id: newLaserIndex + 1, power: 1, readyAt: 0 }];
    requestDraw();
  }
</script>

<svelte:window onpointermove={handlePointerMove} />

<div
  class={[
    'relative touch-manipulation overflow-hidden rounded-lg bg-black select-none',
    contained ? 'h-full w-full' : 'aspect-[5/1] w-[min(82vw,42rem)] max-w-full'
  ]}
>
  <button
    type="button"
    class="absolute inset-0 cursor-crosshair border-0 bg-transparent p-0 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-action"
    aria-label={m['ui.easter_egg.fire']()}
    onclick={triggerBurst}
  >
    <canvas
      class={[
        'pointer-events-none absolute rounded-lg',
        contained
          ? 'inset-0 h-full w-full'
          : 'top-1/2 left-1/2 h-[400%] w-[170%] -translate-x-1/2 -translate-y-1/2'
      ]}
      aria-hidden="true"
      {@attach setupCanvas}
    ></canvas>
  </button>

  <div
    class={[
      'pointer-events-none absolute inset-0 transition-opacity duration-700 ease-out motion-reduce:duration-0',
      gameUiVisible ? 'visible opacity-100' : 'invisible opacity-0'
    ]}
    data-game-ui-visible={gameUiVisible}
    aria-hidden={!gameUiVisible}
  >
    <div
      class="pointer-events-auto absolute top-2 left-2 flex max-w-[72%] flex-wrap items-start gap-1 text-white"
      role="list"
      aria-label={m['ui.easter_egg.laser_guns']({ count: laserGuns.length })}
    >
      {#each laserGuns as laser, index (laser.id)}
        {@const cooldownProgress = laserCooldownProgress(hudNow, laser.readyAt)}
        {@const cooldownSeconds = Math.max(0, (laser.readyAt - hudNow) / 1000).toFixed(1)}
        {@const upgradeCost = laserPowerUpgradeCost(laser.power)}
        <div
          class="flex w-11 flex-col gap-0.5"
          role="listitem"
          data-ready={cooldownProgress >= 1}
          aria-label={cooldownProgress >= 1
            ? m['ui.easter_egg.laser_ready']({
                number: index + 1,
                power: laser.power
              })
            : m['ui.easter_egg.laser_cooldown']({
                number: index + 1,
                power: laser.power,
                seconds: cooldownSeconds
              })}
        >
          <div
            class="flex min-h-10 w-full flex-col items-center justify-center gap-0.5 rounded border border-white/15 bg-black/65 text-xs text-white tabular-nums"
          >
            <span class={cooldownProgress < 1 ? 'opacity-35' : ''} aria-hidden="true"
              >🔫{laser.power}</span
            >
            <span class="h-1 w-7 overflow-hidden rounded-full bg-white/20" aria-hidden="true">
              <span
                class="block h-full rounded-full bg-cyan-300"
                style:width={`${cooldownProgress * 100}%`}
              ></span>
            </span>
          </div>
          <button
            type="button"
            class="min-h-8 w-full cursor-pointer rounded border border-white/20 bg-black/75 px-1 text-[10px] text-white tabular-nums hover:bg-black/90 disabled:cursor-not-allowed disabled:opacity-45"
            disabled={points < upgradeCost}
            aria-label={m['ui.easter_egg.upgrade_power']({
              number: index + 1,
              level: laser.power + 1,
              cost: upgradeCost
            })}
            onclick={() => upgradeLaserPower(index)}>⚡ {upgradeCost}</button
          >
        </div>
      {/each}
    </div>

    <output
      class="pointer-events-none absolute top-2 right-2 rounded bg-black/65 px-2 py-1 font-mono text-sm text-white tabular-nums"
      aria-label={m['ui.easter_egg.points']({ count: points })}>✨ {points}</output
    >

    <div
      class="pointer-events-auto absolute right-2 bottom-2 left-2 flex items-center justify-center gap-2"
    >
      <button
        type="button"
        class="min-h-10 cursor-pointer rounded border border-white/20 bg-black/75 px-2 text-xs text-white tabular-nums hover:bg-black/90 disabled:cursor-not-allowed disabled:opacity-45"
        disabled={laserGuns.length >= MAX_LASER_GUNS || points < nextLaserCost}
        aria-label={laserGuns.length >= MAX_LASER_GUNS
          ? m['ui.easter_egg.maximum_lasers']({ count: MAX_LASER_GUNS })
          : m['ui.easter_egg.buy_laser']({
              number: laserGuns.length + 1,
              cost: nextLaserCost
            })}
        onclick={buyLaserGun}
        >🔫 {laserGuns.length}/{MAX_LASER_GUNS} · {laserGuns.length >= MAX_LASER_GUNS
          ? '⛔'
          : `✨ ${nextLaserCost}`}</button
      >
    </div>
  </div>
</div>
