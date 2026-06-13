import { createContext } from 'svelte';

export type MentionRole = {
  name: string;
  isSystem: boolean;
  position: number;
  pingable: boolean;
};

export type MentionRolesState = {
  roles: MentionRole[];
};

const [getMentionRolesState, setMentionRolesState] = createContext<{
  current: MentionRolesState;
}>();

export function createMentionRoles() {
  const state = $state<{ current: MentionRolesState }>({
    current: {
      roles: []
    }
  });
  setMentionRolesState(state);

  return {
    setRoles(roles: MentionRole[]) {
      state.current.roles = roles;
    },

    clear() {
      state.current.roles = [];
    }
  };
}

export function getMentionRoles(): MentionRole[] {
  return getMentionRolesState().current.roles;
}
