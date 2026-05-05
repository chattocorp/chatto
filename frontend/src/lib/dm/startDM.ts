import { graphql } from '$lib/gql';
import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';
import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { instanceIdToSegment } from '$lib/navigation';

const StartDMMutation = graphql(`
  mutation StartDM($input: StartDMInput!) {
    startDM(input: $input) {
      id
    }
  }
`);

/**
 * Start a DM conversation with a user and navigate to it.
 */
export async function startDMWith(instanceId: string, userId: string): Promise<void> {
  const result = await graphqlClientManager
    .getClient(instanceId)
    .client.mutation(StartDMMutation, { input: { participantIds: [userId] } })
    .toPromise();

  if (result.data?.startDM) {
    goto(
      resolve('/chat/[instanceId]/(chrome)/[roomId]', {
        instanceId: instanceIdToSegment(instanceId),
        roomId: result.data.startDM.id
      })
    );
  }
}
