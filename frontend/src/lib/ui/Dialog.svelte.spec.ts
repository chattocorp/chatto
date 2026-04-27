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

describe('Dialog', () => {
  describe('dialog element', () => {
    it('renders a dialog element', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toBeInTheDocument();
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
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toHaveClass('w-150');
    });

    it('applies small size class when size is sm', async () => {
      const { container } = renderDialog({
        visible: false,
        size: 'sm',
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog')).toHaveClass('w-100');
    });

    it('applies large size class when size is lg', async () => {
      const { container } = renderDialog({
        visible: false,
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

      await expect.element(q(container, 'dialog > div > div.text-text')).toBeInTheDocument();
    });
  });

  describe('styling', () => {
    it('has rounded corners', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog > div')).toHaveClass('rounded-lg');
    });

    it('has padding', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog > div')).toHaveClass('p-6');
    });

    it('has shadow', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog > div')).toHaveClass('shadow-lg');
    });

    it('has border', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      const wrapper = q(container, 'dialog > div');
      await expect.element(wrapper).toHaveClass('border');
      await expect.element(wrapper).toHaveClass('border-border');
    });

    it('has background color', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog > div')).toHaveClass('bg-surface');
    });
  });

  describe('overflow handling', () => {
    it('has vertical overflow auto', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog > div')).toHaveClass('overflow-y-auto');
    });

    it('has max height constraint', async () => {
      const { container } = renderDialog({
        visible: false,
        children: testSnippet('<span>Content</span>')
      });

      await expect.element(q(container, 'dialog > div')).toHaveClass('max-h-[80vh]');
    });
  });

  describe('centering', () => {
    it('is centered with margin auto', async () => {
      const { container } = renderDialog({
        visible: false,
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
