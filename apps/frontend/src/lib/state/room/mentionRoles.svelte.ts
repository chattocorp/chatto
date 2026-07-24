import { createContext } from 'svelte';

export type MentionRole = {
  name: string;
  isSystem: boolean;
  position: number;
  pingable: boolean;
};

const [getMentionRolesState, setMentionRolesState] = createContext<() => MentionRole[]>();

export function createMentionRoles(getRoles: () => MentionRole[] = () => []) {
  setMentionRolesState(getRoles);
}

export function getMentionRoles(): MentionRole[] {
  return getMentionRolesState()();
}
