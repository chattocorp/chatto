<!--
SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
SPDX-License-Identifier: Apache-2.0

@component

Decodes the native helper's local Annex-B H.264 preview onto a canvas. The
frames are the same encoded access units submitted to LiveKit, but never make
a network round trip.
-->
<script lang="ts">
  import type { NativeScreenSharePreview } from '$lib/desktop/nativeScreenSharePublisher';

  let {
    preview,
    name,
    fill = false
  }: { preview: NativeScreenSharePreview; name: string; fill?: boolean } = $props();

  let canvas = $state<HTMLCanvasElement | null>(null);

  function codecForAnnexB(data: Uint8Array): string {
    for (let index = 0; index + 7 < data.length; index += 1) {
      const startCodeLength =
        data[index] === 0 && data[index + 1] === 0 && data[index + 2] === 1
          ? 3
          : data[index] === 0 &&
              data[index + 1] === 0 &&
              data[index + 2] === 0 &&
              data[index + 3] === 1
            ? 4
            : 0;
      if (startCodeLength === 0) continue;
      const nal = index + startCodeLength;
      if ((data[nal] & 0x1f) !== 7 || nal + 3 >= data.length) continue;
      return `avc1.${[data[nal + 1], data[nal + 2], data[nal + 3]]
        .map((value) => value.toString(16).padStart(2, '0'))
        .join('')}`;
    }
    return 'avc1.42e02a';
  }

  $effect(() => {
    const target = canvas;
    const source = preview;
    if (!target || typeof VideoDecoder === 'undefined') return;

    const context = target.getContext('2d', { alpha: false });
    if (!context) return;
    let configured = false;
    let waitingForKeyFrame = true;
    const decoder = new VideoDecoder({
      output(frame) {
        const scale = Math.min(
          target.width / frame.displayWidth,
          target.height / frame.displayHeight
        );
        const width = Math.round(frame.displayWidth * scale);
        const height = Math.round(frame.displayHeight * scale);
        const x = Math.round((target.width - width) / 2);
        const y = Math.round((target.height - height) / 2);
        context.fillStyle = 'black';
        context.fillRect(0, 0, target.width, target.height);
        context.drawImage(frame, x, y, width, height);
        frame.close();
      },
      error(error) {
        console.warn('[Chatto Desktop] Local screen-share preview decoder failed', error);
        configured = false;
        waitingForKeyFrame = true;
      }
    });
    const resize = () => {
      const bounds = target.getBoundingClientRect();
      const scale = Math.min(window.devicePixelRatio || 1, 2);
      target.width = Math.max(1, Math.min(source.width, Math.round(bounds.width * scale)));
      target.height = Math.max(1, Math.min(source.height, Math.round(bounds.height * scale)));
    };
    const resizeObserver = new ResizeObserver(resize);
    resizeObserver.observe(target);
    resize();

    const unsubscribe = source.subscribe((frame) => {
      if (waitingForKeyFrame && !frame.keyFrame) return;
      if (decoder.decodeQueueSize > 8) {
        decoder.reset();
        configured = false;
        waitingForKeyFrame = true;
        if (!frame.keyFrame) return;
      }
      if (!configured) {
        if (!frame.keyFrame) return;
        decoder.configure({
          codec: codecForAnnexB(frame.data),
          hardwareAcceleration: 'prefer-hardware',
          optimizeForLatency: true
        });
        configured = true;
      }
      waitingForKeyFrame = false;
      decoder.decode(
        new EncodedVideoChunk({
          type: frame.keyFrame ? 'key' : 'delta',
          timestamp: frame.timestampUs,
          data: frame.data
        })
      );
    });

    return () => {
      unsubscribe();
      resizeObserver.disconnect();
      decoder.close();
    };
  });
</script>

<div
  class={[
    'relative block w-full overflow-hidden rounded-md bg-black',
    fill ? 'h-full min-h-0' : 'aspect-video'
  ]}
>
  <canvas bind:this={canvas} class="h-full w-full" title={name}></canvas>
</div>
