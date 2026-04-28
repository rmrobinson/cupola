package collector

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rmrobinson/cupola/internal/domain"
)

// Collector is the interface all data-source collectors must implement.
type Collector interface {
	ID() string
	Domain() domain.DomainType
	Start(ctx context.Context) error
	State() domain.DomainState
}

// EmailMessage is delivered to EmailHandler implementations by the IMAP dispatcher.
type EmailMessage struct {
	From    string
	Subject string
	Body    string
	Date    time.Time
}

// EmailHandler is implemented by collectors that accept email-based input
// from the shared IMAP dispatcher.
type EmailHandler interface {
	ID() string
	SenderPatterns() []string  // exact sender addresses
	SubjectPatterns() []string // regex patterns matched against subject
	Handle(msg EmailMessage) error
}

// Registry holds registered collectors, enforcing one per DomainType.
type Registry struct {
	collectors map[domain.DomainType]Collector
}

func NewRegistry() *Registry {
	return &Registry{
		collectors: make(map[domain.DomainType]Collector),
	}
}

// Register adds c to the registry. It fatals if the domain is already taken.
func (r *Registry) Register(c Collector) {
	dt := c.Domain()
	if existing, ok := r.collectors[dt]; ok {
		log.Fatalf("duplicate collector for domain %q: already registered by %s, refusing %s",
			dt, existing.ID(), c.ID())
	}
	r.collectors[dt] = c
}

// Domains returns the DomainType of every registered collector.
func (r *Registry) Domains() []domain.DomainType {
	out := make([]domain.DomainType, 0, len(r.collectors))
	for dt := range r.collectors {
		out = append(out, dt)
	}
	return out
}

// StartAll starts every registered collector. Returns on the first error.
func (r *Registry) StartAll(ctx context.Context) error {
	for _, c := range r.collectors {
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("start collector %s: %w", c.ID(), err)
		}
	}
	return nil
}
