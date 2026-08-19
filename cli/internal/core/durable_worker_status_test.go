package core

import "testing"

func TestDurableWorkerAdminStatusesDeriveAvailabilityAndWork(t *testing.T) {
	statuses := durableWorkerAdminStatuses(&JetStreamStats{Consumers: []ConsumerStats{
		{Stream: "OTHER", Name: assetCleanupConsumerName, Waiting: 1},
		{Stream: "EVT", Name: assetCleanupConsumerName, Waiting: 1, DeliveredStreamSeq: 40, AckFloorStreamSeq: 40},
		{Stream: "EVT", Name: callKeyCleanupConsumerName, Pending: 2, AckPending: 1, Waiting: 1, Redelivered: 3},
		{Stream: "EVT", Name: userKeyShreddingConsumerName},
		{Stream: "EVT", Name: pushSubscriptionCleanupConsumerName, Waiting: 1},
		{Stream: "EVT", Name: notificationWorkerConsumerName, Waiting: 1, DeliveredStreamSeq: 52, AckFloorStreamSeq: 51},
		{Stream: "EVT", Name: assetProcessingConsumerName, Waiting: 0, Pending: 4},
	}}, false)

	byKey := make(map[string]DurableWorkerAdminStatus, len(statuses))
	for _, status := range statuses {
		byKey[status.Key] = status
	}
	if got := byKey["asset_cleanup"]; got.Health != DurableWorkerHealthHealthy || got.AckFloorSequence != 40 {
		t.Fatalf("asset cleanup status = %+v", got)
	}
	if got := byKey["call_key_cleanup"]; got.Health != DurableWorkerHealthWorking || got.PendingCount != 2 || got.AckPendingCount != 1 || got.RedeliveredCount != 3 {
		t.Fatalf("call key cleanup status = %+v", got)
	}
	if got := byKey["user_key_shredding"]; got.Health != DurableWorkerHealthStalled {
		t.Fatalf("user key shredding status = %+v, want stalled", got)
	}
	if got := byKey["user_push_subscription_cleanup"]; got.Health != DurableWorkerHealthHealthy {
		t.Fatalf("user push-subscription cleanup status = %+v, want healthy", got)
	}
	if got := byKey["notification_materializer"]; got.Health != DurableWorkerHealthHealthy || got.LastDeliveredSequence != 52 || got.AckFloorSequence != 51 {
		t.Fatalf("notification materializer status = %+v, want healthy with consumer progress", got)
	}
	if got := byKey["asset_processing"]; got.Health != DurableWorkerHealthInactive {
		t.Fatalf("asset processing status = %+v, want inactive", got)
	}
}

func TestDurableWorkerAdminStatusesDoNotInferHandlerLivenessFromAckPending(t *testing.T) {
	statuses := durableWorkerAdminStatuses(&JetStreamStats{Consumers: []ConsumerStats{
		{Stream: "EVT", Name: assetCleanupConsumerName, AckPending: 1},
	}}, false)
	if got := statuses[0]; got.Health != DurableWorkerHealthUnconfirmed || got.AckPendingCount != 1 {
		t.Fatalf("asset cleanup status = %+v, want unconfirmed", got)
	}
}

func TestDurableWorkerAdminStatusesReportMissingRequiredConsumers(t *testing.T) {
	statuses := durableWorkerAdminStatuses(nil, true)
	if len(statuses) != 7 {
		t.Fatalf("statuses len = %d, want 7", len(statuses))
	}
	for _, status := range statuses {
		if status.Health != DurableWorkerHealthUnavailable {
			t.Fatalf("%s health = %v, want unavailable", status.Key, status.Health)
		}
	}
}
