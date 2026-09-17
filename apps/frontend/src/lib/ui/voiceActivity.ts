import type { Attachment } from 'svelte/attachments';
import { microphoneMeter } from '$lib/audio/noiseGate';
import { paintVoiceFog } from './voiceFog';

type VoiceLayer = {
  sample: () => void;
  draw: (time: number) => boolean;
};

// One clock for all mounted layers. Silent cards need samples, but no animation frames.
const layers = new Set<VoiceLayer>();
let timer: ReturnType<typeof setInterval> | undefined;
let frame = 0;

function animate(time: number) {
  frame = 0;
  let moving = false;
  for (const layer of layers) moving = layer.draw(time) || moving;
  if (moving) frame = requestAnimationFrame(animate);
}

function sample() {
  if (document.hidden) {
    cancelAnimationFrame(frame);
    frame = 0;
    return;
  }
  for (const layer of layers) layer.sample();
  if (!frame) animate(performance.now());
}

/**
 * Paint an audio-level glow without reactive updates to its containing card.
 * Levels are normalized amplitudes (0–1), mapped to perceived loudness.
 * Detach releases the shared clock; hidden canvases do no work. Reduced motion
 * keeps the fog texture fixed and updates intensity without animation.
 */
export function voiceActivity(readLevel: () => number): Attachment<HTMLCanvasElement> {
  return (canvas) => {
    const context = canvas.getContext('2d');
    if (!context) return;
    const seed = Math.floor(Math.random() * 0xffffffff);
    const texture = document.createElement('canvas');
    const textureContext = texture.getContext('2d');
    if (!textureContext) return;
    let pixels: ImageData;
    let travel = 0;
    let textureTime = -Infinity;
    const motion = matchMedia('(prefers-reduced-motion: reduce)');
    let visible = false;
    let width = 0;
    let height = 0;
    let target = 0;
    let level = 0;
    let previousTime = 0;
    let painted = false;
    let color = '';

    function clear() {
      context!.clearRect(0, 0, width, height);
      painted = false;
      canvas.dataset.active = 'false';
    }

    const resize = new ResizeObserver(([entry]) => {
      width = entry.contentRect.width;
      height = entry.contentRect.height;
      const scale = Math.min(devicePixelRatio || 1, 2);
      canvas.width = Math.round(width * scale);
      canvas.height = Math.round(height * scale);
      context.setTransform(scale, 0, 0, scale, 0, 0);
      texture.width = Math.min(160, Math.max(64, Math.ceil(width / 3)));
      texture.height = 32;
      pixels = textureContext.createImageData(texture.width, texture.height);
      textureTime = -Infinity;
    });
    const intersection = new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting;
      if (!visible) {
        level = target = previousTime = 0;
        clear();
      } else sample();
    });

    const layer: VoiceLayer = {
      sample() {
        if (!visible) return;
        const value = readLevel();
        // The microphone meter's decibel scale keeps quiet speech visible.
        target = Number.isFinite(value) ? microphoneMeter(Math.max(0, value)) : 0;
        if (target || level) color = getComputedStyle(canvas).color;
      },
      draw(time) {
        if (!visible || !width || !height) return false;
        const elapsed = previousTime ? Math.min(time - previousTime, 64) : 16;
        previousTime = time;
        level = motion.matches
          ? target
          : level + (target - level) * (1 - Math.exp(-elapsed / (target > level ? 75 : 220)));
        if (target === 0 && level < 0.002) {
          level = 0;
          if (painted) clear();
          return false;
        }

        context.clearRect(0, 0, width, height);
        canvas.dataset.active = 'true';
        painted = true;
        const strength = Math.sqrt(level);
        // Integrate speed from the smoothed level. Scaling absolute time instead
        // would jump to a different point on the noise path when volume changes.
        if (!motion.matches) travel += elapsed * (0.3 + level * 1.7);
        // Work on a bounded, low-resolution mask at 30 Hz, then let the canvas
        // interpolate it. Reduced motion reuses one still texture.
        if (textureTime === -Infinity || (!motion.matches && time - textureTime >= 1000 / 30)) {
          paintVoiceFog(pixels.data, texture.width, texture.height, travel / 1000, seed);
          textureContext.putImageData(pixels, 0, 0);
          textureTime = time;
        }
        const spread = height * (0.65 + strength * 0.35);
        context.save();
        context.globalAlpha = strength;
        context.drawImage(texture, 0, (height - spread) / 2, width, spread);
        context.globalCompositeOperation = 'source-in';
        context.globalAlpha = 1;
        context.fillStyle = color;
        context.fillRect(0, 0, width, height);
        context.restore();
        return !motion.matches;
      }
    };

    canvas.dataset.active = 'false';
    layers.add(layer);
    resize.observe(canvas);
    intersection.observe(canvas);
    if (layers.size === 1) {
      timer = setInterval(sample, 60);
      document.addEventListener('visibilitychange', sample);
    }
    motion.addEventListener('change', sample);
    return () => {
      resize.disconnect();
      intersection.disconnect();
      motion.removeEventListener('change', sample);
      layers.delete(layer);
      if (layers.size === 0) {
        clearInterval(timer);
        timer = undefined;
        cancelAnimationFrame(frame);
        frame = 0;
        document.removeEventListener('visibilitychange', sample);
      }
    };
  };
}
