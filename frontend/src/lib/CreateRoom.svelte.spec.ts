import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Client } from '@urql/svelte';
import CreateRoom from './CreateRoom.svelte';
import { createMockConnection, createMockGraphqlClient, q } from '$lib/test-utils';

const mutationData = {
  createRoom: { id: 'room_123', name: 'Test Room', description: 'Test' }
};

vi.mock('$lib/state/instance/connection.svelte', () => ({
  useConnection: () => () => createMockConnection({ mutationData })
}));

function renderCreateRoom(props: { spaceId: string }, context: Map<string, unknown>) {
  return render(CreateRoom, { props, context });
}

describe('CreateRoom', () => {
  let mockClient: Client;

  beforeEach(() => {
    mockClient = createMockGraphqlClient({ mutationData });
  });

  describe('form rendering', () => {
    it('renders the room name input', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, '#room-name')).toBeInTheDocument();
    });

    it('renders the description textarea', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, '#room-description')).toBeInTheDocument();
    });

    it('renders the submit button', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      const button = q(container, 'button[type="submit"]');
      await expect.element(button).toBeInTheDocument();
      await expect.element(button).toHaveTextContent('Create Room');
    });

    it('button is disabled when name is empty', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      await expect.element(q(container, 'button[type="submit"]')).toBeDisabled();
    });
  });

  describe('form labels', () => {
    it('has label for room name', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      const label = q(container, 'label[for="room-name"]');
      await expect.element(label).toBeInTheDocument();
      await expect.element(label).toHaveTextContent('Room Name');
    });

    it('has label for description', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      const label = q(container, 'label[for="room-description"]');
      await expect.element(label).toBeInTheDocument();
      await expect.element(label).toHaveTextContent('Description');
    });
  });

  describe('input placeholders', () => {
    it('name input has placeholder', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, '#room-name'))
        .toHaveAttribute('placeholder', 'Enter room name');
    });

    it('description textarea has placeholder', async () => {
      const { container } = renderCreateRoom(
        { spaceId: 'space_123' },
        new Map([['$$_urql', mockClient]])
      );

      await expect
        .element(q(container, '#room-description'))
        .toHaveAttribute('placeholder', "What's this room about?");
    });
  });
});
