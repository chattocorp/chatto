import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { dropZone } from './dropZone.svelte';

/**
 * Build a DragEvent that mimics a real file drag.
 * `dataTransfer.types` must include 'Files' for the drop zone to engage.
 */
function dragEvent(
  type: 'dragenter' | 'dragover' | 'dragleave' | 'drop',
  options: { files?: File[]; types?: string[] } = {}
): DragEvent {
  const types = options.types ?? ['Files'];
  const files = options.files ?? [];

  // FileList is awkward to construct directly; use a DataTransfer-like stub.
  const fakeFiles = Object.assign(files, {
    item: (i: number) => files[i] ?? null
  }) as unknown as FileList;

  const dt = {
    types,
    files: fakeFiles,
    dropEffect: 'none' as DataTransfer['dropEffect']
  } as unknown as DataTransfer;

  const event = new Event(type, { bubbles: true, cancelable: true }) as DragEvent;
  Object.defineProperty(event, 'dataTransfer', { value: dt });
  return event;
}

function file(name: string, type: string): File {
  return new File([new Uint8Array([0])], name, { type });
}

describe('dropZone attachment', () => {
  let host: HTMLDivElement;
  let cleanup: (() => void) | undefined;

  beforeEach(() => {
    host = document.createElement('div');
    document.body.appendChild(host);
  });

  afterEach(() => {
    cleanup?.();
    cleanup = undefined;
    host.remove();
  });

  function attach(opts: Parameters<typeof dropZone>[0]) {
    cleanup = dropZone(opts)(host);
  }

  describe('drag state', () => {
    it('fires onDragStateChange(true) on first dragenter', () => {
      const onDrop = vi.fn();
      const onDragStateChange = vi.fn();
      attach({ onDrop, onDragStateChange });

      host.dispatchEvent(dragEvent('dragenter'));
      expect(onDragStateChange).toHaveBeenCalledWith(true);
      expect(onDragStateChange).toHaveBeenCalledOnce();
    });

    it('does not fire again on nested dragenter (counter handles re-entries)', () => {
      const onDragStateChange = vi.fn();
      attach({ onDrop: () => {}, onDragStateChange });

      host.dispatchEvent(dragEvent('dragenter'));
      host.dispatchEvent(dragEvent('dragenter'));
      host.dispatchEvent(dragEvent('dragenter'));

      expect(onDragStateChange).toHaveBeenCalledTimes(1);
      expect(onDragStateChange).toHaveBeenLastCalledWith(true);
    });

    it('only fires onDragStateChange(false) when the counter returns to zero', () => {
      const onDragStateChange = vi.fn();
      attach({ onDrop: () => {}, onDragStateChange });

      host.dispatchEvent(dragEvent('dragenter'));
      host.dispatchEvent(dragEvent('dragenter'));
      host.dispatchEvent(dragEvent('dragleave'));
      expect(onDragStateChange).toHaveBeenCalledTimes(1); // still dragging

      host.dispatchEvent(dragEvent('dragleave'));
      expect(onDragStateChange).toHaveBeenLastCalledWith(false);
      expect(onDragStateChange).toHaveBeenCalledTimes(2);
    });

    it('ignores non-file drags (no Files in types)', () => {
      const onDragStateChange = vi.fn();
      attach({ onDrop: () => {}, onDragStateChange });

      host.dispatchEvent(dragEvent('dragenter', { types: ['text/plain'] }));
      expect(onDragStateChange).not.toHaveBeenCalled();
    });

    it('preventDefault on dragenter so the browser allows the drop', () => {
      attach({ onDrop: () => {} });
      const ev = dragEvent('dragenter');
      host.dispatchEvent(ev);
      expect(ev.defaultPrevented).toBe(true);
    });

    it('sets dropEffect to "copy" on dragover', () => {
      attach({ onDrop: () => {} });
      const ev = dragEvent('dragover');
      host.dispatchEvent(ev);
      expect(ev.dataTransfer?.dropEffect).toBe('copy');
      expect(ev.defaultPrevented).toBe(true);
    });
  });

  describe('drop & MIME filtering', () => {
    it('forwards files matching the default image/* filter', () => {
      const onDrop = vi.fn();
      attach({ onDrop });

      const png = file('a.png', 'image/png');
      host.dispatchEvent(dragEvent('drop', { files: [png] }));

      expect(onDrop).toHaveBeenCalledOnce();
      expect(onDrop.mock.calls[0][0]).toEqual([png]);
    });

    it('drops files that do not match the accepted types', () => {
      const onDrop = vi.fn();
      attach({ onDrop, acceptedTypes: ['image/*'] });

      const pdf = file('doc.pdf', 'application/pdf');
      host.dispatchEvent(dragEvent('drop', { files: [pdf] }));

      expect(onDrop).not.toHaveBeenCalled();
    });

    it('filters a mixed drop down to the accepted files only', () => {
      const onDrop = vi.fn();
      attach({ onDrop, acceptedTypes: ['image/*'] });

      const png = file('a.png', 'image/png');
      const pdf = file('doc.pdf', 'application/pdf');
      const jpg = file('b.jpg', 'image/jpeg');
      host.dispatchEvent(dragEvent('drop', { files: [png, pdf, jpg] }));

      expect(onDrop).toHaveBeenCalledOnce();
      expect(onDrop.mock.calls[0][0]).toEqual([png, jpg]);
    });

    it('matches exact MIME types when the pattern has no wildcard', () => {
      const onDrop = vi.fn();
      attach({ onDrop, acceptedTypes: ['application/pdf'] });

      const pdf = file('doc.pdf', 'application/pdf');
      const png = file('a.png', 'image/png');
      host.dispatchEvent(dragEvent('drop', { files: [pdf, png] }));

      expect(onDrop.mock.calls[0][0]).toEqual([pdf]);
    });

    it('does not call onDrop when no files in the drop match', () => {
      const onDrop = vi.fn();
      attach({ onDrop });
      host.dispatchEvent(dragEvent('drop', { files: [] }));
      expect(onDrop).not.toHaveBeenCalled();
    });

    it('resets drag state to "not dragging" on drop', () => {
      const onDragStateChange = vi.fn();
      attach({ onDrop: () => {}, onDragStateChange });

      host.dispatchEvent(dragEvent('dragenter'));
      onDragStateChange.mockClear();

      const png = file('a.png', 'image/png');
      host.dispatchEvent(dragEvent('drop', { files: [png] }));
      expect(onDragStateChange).toHaveBeenCalledWith(false);
    });

    it('preventDefault on drop so the browser does not navigate', () => {
      attach({ onDrop: () => {} });
      const ev = dragEvent('drop', { files: [file('a.png', 'image/png')] });
      host.dispatchEvent(ev);
      expect(ev.defaultPrevented).toBe(true);
    });
  });

  describe('cleanup', () => {
    it('removes all listeners when the cleanup function runs', () => {
      const onDrop = vi.fn();
      const onDragStateChange = vi.fn();
      attach({ onDrop, onDragStateChange });

      cleanup?.();
      cleanup = undefined;

      host.dispatchEvent(dragEvent('dragenter'));
      host.dispatchEvent(dragEvent('drop', { files: [file('a.png', 'image/png')] }));

      expect(onDrop).not.toHaveBeenCalled();
      expect(onDragStateChange).not.toHaveBeenCalled();
    });
  });
});
