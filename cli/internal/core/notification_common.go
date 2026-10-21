package core

import "time"

const (
	notificationTTL                  = 90 * 24 * time.Hour
	notificationReadRetention        = 7 * 24 * time.Hour
	notificationPhysicalCleanupGrace = 24 * time.Hour
)
