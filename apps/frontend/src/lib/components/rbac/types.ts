/**
 * Shared types for RBAC components
 */

export type Role = {
  name: string;
  displayName: string;
  description: string;
  isSystem: boolean;
  position: number;
  pingable: boolean;
};

export type PermissionState = 'allow' | 'deny' | 'neutral';
