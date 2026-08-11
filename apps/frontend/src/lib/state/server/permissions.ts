/** Viewer permissions returned by the server's viewer API. */
export type ViewerData = {
  /** Whether the viewer has at least one admin-capability entry point. */
  canViewAdmin: boolean;
  canStartDMs: boolean;
  canAdminViewUsers: boolean;
  canAdminManageAccounts: boolean;
  canAssignRoles: boolean;
  canAdminViewRoles: boolean;
  canAdminManageRoles: boolean;
  canAdminViewSystem: boolean;
  canAdminViewAudit: boolean;
  canManageInvites: boolean;
};

/** Canonical per-server viewer permissions and their load state. */
export type ServerPermissions = ViewerData & {
  loaded: boolean;
};
