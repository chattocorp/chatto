import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import RoleForm from './RoleForm.svelte';

// Type helper - vitest-browser-svelte handles target internally but types don't reflect this
function renderRoleForm(
  props: Partial<{
    name: string;
    displayName: string;
    description: string;
    nameEditable: boolean;
    saving: boolean;
    submitLabel: string;
    savingLabel: string;
    onSubmit: () => void;
    onCancel: () => void;
  }>
) {
  const defaultProps = {
    name: '',
    displayName: '',
    description: '',
    nameEditable: true,
    saving: false,
    submitLabel: 'Save',
    savingLabel: 'Saving...',
    onSubmit: vi.fn(),
    ...props
  };
  return render(RoleForm, { props: defaultProps });
}

import { q } from '$lib/test-utils';

describe('RoleForm', () => {
  describe('form rendering', () => {
    it('renders name input when nameEditable is true', async () => {
      const { container } = renderRoleForm({ nameEditable: true });
      await expect.element(q(container, '#name')).toBeInTheDocument();
    });

    it('renders name as code when nameEditable is false', async () => {
      const { container } = renderRoleForm({ nameEditable: false, name: 'testrole' });
      await expect.element(q(container, '#name')).not.toBeInTheDocument();
      await expect.element(q(container, 'code')).toHaveTextContent('testrole');
    });

    it('renders display name input', async () => {
      const { container } = renderRoleForm({});
      await expect.element(q(container, '#displayName')).toBeInTheDocument();
    });

    it('renders description textarea', async () => {
      const { container } = renderRoleForm({});
      await expect.element(q(container, '#description')).toBeInTheDocument();
    });

    it('renders submit button with custom label', async () => {
      const { container } = renderRoleForm({ submitLabel: 'Create Role' });
      const button = q(container, 'button[type="submit"]');
      await expect.element(button).toHaveTextContent('Create Role');
    });

    it('renders cancel button when onCancel is provided', async () => {
      const { container } = renderRoleForm({ onCancel: vi.fn() });
      const cancelButton = container.querySelector('button:not([type="submit"])');
      await expect.element(cancelButton as HTMLElement).toHaveTextContent('Cancel');
    });

    it('does not render cancel button when onCancel is not provided', async () => {
      const { container } = renderRoleForm({});
      const buttons = container.querySelectorAll('button');
      expect(buttons.length).toBe(1); // Only submit button
    });
  });

  describe('form validation', () => {
    it('submit button is disabled when name is empty', async () => {
      const { container } = renderRoleForm({ name: '', displayName: 'Test' });
      await expect.element(q(container, 'button[type="submit"]')).toBeDisabled();
    });

    it('submit button is disabled when displayName is empty', async () => {
      const { container } = renderRoleForm({ name: 'test', displayName: '' });
      await expect.element(q(container, 'button[type="submit"]')).toBeDisabled();
    });

    it('submit button is enabled when both name and displayName are filled', async () => {
      const { container } = renderRoleForm({ name: 'test', displayName: 'Test Role' });
      await expect.element(q(container, 'button[type="submit"]')).toBeEnabled();
    });

    it('shows error for invalid name format', async () => {
      const { container } = renderRoleForm({ name: 'InvalidName', displayName: 'Test' });
      await expect.element(q(container, 'p.text-error')).toBeInTheDocument();
      expect(container.textContent).toContain('lowercase');
    });

    it('shows error for name starting with number', async () => {
      const { container } = renderRoleForm({ name: '1invalid', displayName: 'Test' });
      await expect.element(q(container, 'p.text-error')).toBeInTheDocument();
      expect(container.textContent).toContain('start with a letter');
    });

    it('accepts valid name format', async () => {
      const { container } = renderRoleForm({ name: 'validrole', displayName: 'Test' });
      // Should not have error text
      const error = q(container, 'p.text-error');
      expect(error).toBeNull();
    });

    it('accepts name with dashes and numbers', async () => {
      const { container } = renderRoleForm({ name: 'self-hosters-2', displayName: 'Test' });
      const error = q(container, 'p.text-error');
      expect(error).toBeNull();
    });
  });

  describe('saving state', () => {
    it('disables inputs when saving', async () => {
      const { container } = renderRoleForm({
        name: 'test',
        displayName: 'Test',
        saving: true
      });
      await expect.element(q(container, '#name')).toBeDisabled();
      await expect.element(q(container, '#displayName')).toBeDisabled();
      await expect.element(q(container, '#description')).toBeDisabled();
    });

    it('disables submit button when saving', async () => {
      const { container } = renderRoleForm({
        name: 'test',
        displayName: 'Test',
        saving: true
      });
      await expect.element(q(container, 'button[type="submit"]')).toBeDisabled();
    });
  });
});
