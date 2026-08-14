package traceinventory

import (
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// SourceFilter selects which evidence owners participate in an inventory.
type SourceFilter string

const (
	SourceFilterAll      SourceFilter = "ALL"
	SourceFilterTarget   SourceFilter = "TARGET"
	SourceFilterImported SourceFilter = "IMPORTED"
)

// Query is one finite inventory request. A zero filter means ALL and a zero
// page size selects the service default.
type Query struct {
	SourceFilter SourceFilter
	PageSize     int
	Continuation string
}

// ApplicationCatalog reports whether the selected target's catalog was
// requested and available independently from locally installed evidence.
type ApplicationCatalog struct {
	Requested     bool
	Available     bool
	TargetScopeID string
	InstanceID    string
	Error         *consolecore.Error
}

// Entry is one target- or import-owned trace inventory item. Target catalog
// facts and local installed-copy facts deliberately remain separate.
type Entry struct {
	Source                    evidence.Source
	TargetScopeID             string
	TraceID                   string
	SessionID                 string
	EntrySkill                string
	Outcome                   string
	FinalizedAt               time.Time
	SizeBytes                 int64
	PersistencePolicy         string
	ApplicationTraceExpiresAt *time.Time
	ApplicationAvailability   string
	LocalAvailable            bool
	ArtifactHandle            artifact.Handle
	AcquiredAt                time.Time
	LastUsedAt                time.Time
	LocalExpiresAt            time.Time
	HasIdleExpiry             bool
	LocalBytes                int64
}

// Result is one finite inventory page.
type Result struct {
	ObservedAt         time.Time
	ApplicationCatalog ApplicationCatalog
	Items              []Entry
	HasMore            bool
	Continuation       string
}
