package domain

// AlertSeverity is shared across weather, transit, and municipal alert types.
type AlertSeverity string

const (
	SeverityInfo      AlertSeverity = "info"
	SeverityWatch     AlertSeverity = "watch"
	SeverityWarning   AlertSeverity = "warning"
	SeverityEmergency AlertSeverity = "emergency"
)
