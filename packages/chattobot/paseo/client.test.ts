import { expect, test, vi } from 'vitest';
import { createPaseoClient } from './client.ts';

const sdk = vi.hoisted(() => ({
  connect: vi.fn(),
  close: vi.fn(),
  dispose: vi.fn(),
  setWorkspaceLabel: vi.fn()
}));
vi.mock('@getpaseo/client/internal/daemon-client', () => ({
  DaemonClient: class {
    connect = sdk.connect;
    close = sdk.close;
    setWorkspaceLabel = sdk.setWorkspaceLabel;
  }
}));
vi.mock('@getpaseo/client', () => ({ createPaseoApi: () => ({ dispose: sdk.dispose }) }));

test('assigns the fixed workspace label and checks the acknowledged assignment', async () => {
  const client = createPaseoClient({ url: 'ws://fixture/ws' });
  sdk.setWorkspaceLabel.mockResolvedValue({ workspaceLabels: ['existing', 'chattobot'] });
  expect(await client.labelWorkspace('workspace')).toBe(true);
  expect(sdk.setWorkspaceLabel).toHaveBeenCalledExactlyOnceWith({
    workspaceId: 'workspace',
    label: { name: 'chattobot', color: 'emerald' },
    assigned: true
  });
  sdk.setWorkspaceLabel.mockResolvedValue({ workspaceLabels: ['existing'] });
  expect(await client.labelWorkspace('workspace')).toBe(false);
});

test('closes the transport even if subscription disposal fails', async () => {
  const client = createPaseoClient({ url: 'ws://fixture/ws' });
  sdk.dispose.mockRejectedValueOnce(new Error('Disposal failed'));
  await expect(client.close()).rejects.toThrow('Disposal failed');
  expect(sdk.close).toHaveBeenCalledOnce();
});
