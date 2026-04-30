import { describe, it, expect } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Dialog from './Dialog.svelte';
import { q, testSnippet } from '$lib/test-utils';

function renderDialog(props: {
  visible: boolean;
  title?: string;
  size?: 'sm' | 'md' | 'lg';
  children: ReturnType<typeof testSnippet>;
}) {
  return render(Dialog, { props });
}

const FRAME = 'dialog > div';
const WELL = 'dialog > div > div';

describe('Dialog', () => {
  describe('dialog element', () => {
    it('renders a dialog element', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toBeInTheDocument();
    });

    it('does not render contents when closed', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      // Dialog stays in the DOM but its content tree is gated on `visible`.
      expect(q(container, FRAME)).toBeNull();
    });
  });

  describe('title', () => {
    it('renders title when provided', async () => {
      const { container } = renderDialog({
        visible: true,
        title: 'Test Dialog Title',
        children: testSnippet('<span>Content</span>')
      });

      const title = q(container, 'h2');
      await expect.element(title).toBeInTheDocument();
      await expect.element(title).toHaveTextContent('Test Dialog Title');
    });

    it('does not render title when not provided', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      expect(q(container, 'h2')).toBeNull();
    });
  });

  describe('size classes', () => {
    it('applies medium size class by default', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toHaveClass('w-150');
    });

    it('applies small size class when size is sm', async () => {
      const { container } = renderDialog({
        visible: true,
        size: 'sm',
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toHaveClass('w-100');
    });

    it('applies large size class when size is lg', async () => {
      const { container } = renderDialog({
        visible: true,
        size: 'lg',
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toHaveClass('w-200');
    });
  });

  describe('content area', () => {
    it('has content wrapper div', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Test Content</span>')
      });

      await expect.element(q(container, `${WELL} > div.text-text`)).toBeInTheDocument();
    });
  });

  describe('frame styling', () => {
    it('outer frame uses surface-100 with subtle border and shadow', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      const frame = q(container, FRAME);
      await expect.element(frame).toHaveClass('bg-surface-100');
      await expect.element(frame).toHaveClass('border');
      await expect.element(frame).toHaveClass('rounded-lg');
      await expect.element(frame).toHaveClass('shadow-xl');
    });
  });

  describe('well styling', () => {
    it('inner well sits on the page background color', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      const well = q(container, WELL);
      await expect.element(well).toHaveClass('bg-background');
      await expect.element(well).toHaveClass('rounded-md');
    });

    it('well has padding', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, WELL)).toHaveClass('p-3');
    });
  });

  describe('overflow handling', () => {
    it('well has vertical overflow auto', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, WELL)).toHaveClass('overflow-y-auto');
    });
  });

  describe('centering', () => {
    it('is centered with margin auto', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toHaveClass('m-auto');
    });
  });

  describe('close button', () => {
    it('has a close button with aria-label', async () => {
      const { container } = renderDialog({
        visible: true,
        children: testSnippet('<span>Content</span>')
      });

      const closeButton = q(container, 'button[aria-label="Close"]');
      await expect.element(closeButton).toBeInTheDocument();
    });
  });
});
