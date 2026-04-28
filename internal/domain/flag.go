package domain

import "time"

type FlagStatus struct {
	StateBase
	AtHalfMast bool       `json:"at_half_mast"`
	Reason     *string    `json:"reason,omitempty"`
	Since      *time.Time `json:"since,omitempty"`
	Until      *time.Time `json:"until,omitempty"`
	SourceURL  string     `json:"source_url"`
}

func (FlagStatus) DomainType() DomainType { return DomainFlagStatus }
