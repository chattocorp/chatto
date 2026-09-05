package evtstream

import "testing"

func TestRBACDMAggregateUsesStableLane(t *testing.T) {
	aggregate := RBACDMAggregate()
	if got := aggregate.Subject(EventRBACPermissionGranted); got != "evt.rbac.dm.permission_granted" {
		t.Fatalf("DM RBAC subject = %q, want stable DM lane", got)
	}
}
