package artifact

import (
	"context"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/evidence"
)

// ApplicationAvailability records the last observed upstream availability of a
// finalized trace at the moment its artifact was acquired. It is distinct from
// local-handle availability, which is whether an installed copy exists in the
// current target scope.
type ApplicationAvailability string

const (
	// ApplicationAvailable means the trace was reachable and authorized when
	// its artifact was acquired.
	ApplicationAvailable ApplicationAvailability = "AVAILABLE"
	// ApplicationUnavailable means the trace could not be reached or authorized
	// during the last observation.
	ApplicationUnavailable ApplicationAvailability = "UNAVAILABLE"
)

// TraceMetadata is the immutable acquisition-time trace metadata copied from the
// authoritative observability response. It is retained by the entry for the
// entry's entire lifetime so installed evidence remains usable after upstream
// authentication failure or application expiry.
type TraceMetadata struct {
	TraceID                   string
	SessionID                 string
	EntrySkill                string
	Outcome                   string
	FinalizedAt               time.Time
	SizeBytes                 int64
	PersistencePolicy         string
	ApplicationTraceExpiresAt time.Time
}

// AcquiredArtifact is the result of a successful acquisition. It carries the
// opaque handle and the immutable metadata a caller needs without ever
// exposing a filesystem path.
type AcquiredArtifact struct {
	Owner         evidence.Owner
	Handle        Handle
	Metadata      TraceMetadata
	LocalBytes    int64
	AcquiredAt    time.Time
	LastUsedAt    time.Time
	ExpiresAt     time.Time
	HasIdleExpiry bool
}

// entryState tracks the lifecycle of a single (scope, trace) entry.
type entryState int

const (
	stateAcquiring entryState = iota
	stateInstalled
	stateDeferredRemoval
	stateRemoved
)

// entryKey uniquely identifies an artifact within the service by target scope
// and trace ID.
type entryKey struct {
	owner   evidence.Owner
	traceID string
}

// entry is the internal representation of one acquired artifact. It is never
// exposed outside the package; callers interact through handles, leases, and
// DTOs that never carry the installed path.
type entry struct {
	key      entryKey
	handle   Handle
	state    entryState
	metadata TraceMetadata

	// installedDir is the absolute path to the installed bundle directory
	// beneath the verified transient subtree. It contains the raw NDJSON
	// component and processor-created derived components. It is never returned
	// to callers.
	installedDir string
	// localBytes is the aggregate raw plus derived byte count charged to
	// capacity for the complete bundle.
	localBytes int64
	// rawBytes is the exact installed raw artifact byte count. It is a component
	// of localBytes and is retained for raw-component addressing.
	rawBytes int64
	// componentSizes maps each derived component name to its final synced byte
	// count. The raw artifact (ComponentRawArtifact) is tracked via rawBytes.
	componentSizes map[ComponentName]int64

	// acquisitionTime is when the entry was created.
	acquisitionTime time.Time
	// lastUsedAt is the time of the last successful lease close. It is the
	// origin for idle TTL expiry.
	lastUsedAt time.Time

	// pinCount is the number of active leases. A non-zero pin count prevents
	// eviction and explicit removal. Expiry during a pin defers deletion.
	pinCount int
	leases   map[*Lease]struct{}

	// applicationAvailability is the last observed upstream availability.
	applicationAvailability ApplicationAvailability

	// Acquisition coordination fields. Only used while state == stateAcquiring.
	acquireCtx      context.Context
	acquireCancel   context.CancelFunc
	scopeStop       func() bool // stops watching scope context cancellation
	acquireResult   acquireResult
	acquireDone     chan struct{} // closed when the acquisition result is available
	acquireFinished chan struct{} // closed when the leader goroutine exits
	waiters         int           // number of callers waiting for the result
}

// acquireResult carries the outcome of an acquisition to all waiters.
type acquireResult struct {
	artifact AcquiredArtifact
	err      *consolecore.Error
}

// LookupResult is a read-only view of one installed artifact entry, looked up
// by trace ID within the current scope. It carries the opaque handle and
// availability facts without exposing a filesystem path.
type LookupResult struct {
	Owner                   evidence.Owner
	Handle                  Handle
	Metadata                TraceMetadata
	LocalAvailable          bool
	ApplicationAvailability ApplicationAvailability
	AcquiredAt              time.Time
	LastUsedAt              time.Time
	ExpiresAt               time.Time
	HasIdleExpiry           bool
	LocalBytes              int64
}

// StorageSnapshot is a side-effect-free view of the artifact cache. Viewing it
// does not refresh any entry's last-use time.
type StorageSnapshot struct {
	WorkspaceLabel string
	MaxBytes       int64
	Unlimited      bool
	IdleTTL        time.Duration
	NeverExpire    bool
	ChargedBytes   int64
	AcquiredCount  int
	Entries        []StoredEntry
}

// StoredEntry is one entry in a StorageSnapshot. It carries cache facts without
// exposing a filesystem path or the raw opaque handle.
type StoredEntry struct {
	Source                    evidence.Source `json:"source"`
	TargetScopeID             string          `json:"targetScopeId,omitempty"`
	TraceID                   string          `json:"traceId"`
	SessionID                 string          `json:"sessionId"`
	Outcome                   string          `json:"outcome"`
	PersistencePolicy         string          `json:"persistencePolicy"`
	FinalizedAt               time.Time       `json:"finalizedAt"`
	AcquiredAt                time.Time       `json:"acquiredAt"`
	LastUsedAt                time.Time       `json:"lastUsedAt"`
	ExpiresAt                 time.Time       `json:"expiresAt"`
	HasIdleExpiry             bool            `json:"hasIdleExpiry"`
	LocalBytes                int64           `json:"localBytes"`
	ApplicationTraceExpiresAt *time.Time      `json:"applicationTraceExpiresAt,omitempty"`
	ApplicationAvailability   string          `json:"applicationAvailability,omitempty"`
	LocalAvailable            bool            `json:"localAvailable"`
	ActivePin                 bool            `json:"activePin"`
}
