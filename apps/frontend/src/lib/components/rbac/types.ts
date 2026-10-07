/**
 * Shared types for RBAC components
 */

export type Role = {
  name: string;
  displayName: string;
  description: string;
  permissions: string[];
  permissionDenials: string[];
  isSystem: boolean;
  position: number;
  pingable: boolean;
  /**
   * True when the role ranks below the viewer's highest role. The role order
   * lets the viewer edit, delete, assign, or move only such roles.
   */
  ranksBelowViewer: boolean;
};

export type PermissionState = 'allow' | 'deny' | 'neutral';
