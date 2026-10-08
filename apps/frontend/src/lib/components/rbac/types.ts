/**
 * Shared types for RBAC components
 */

export type Role = {
  name: string;
  displayName: string;
  description: string;
  /** Permissions that the role grants at server scope. */
  permissions: string[];
  isSystem: boolean;
  pingable: boolean;
};

export type PermissionState = 'allow' | 'deny' | 'neutral';
