package traceinventory

import "time"

type EvidenceSource string

const (
	SourceTarget   EvidenceSource = "TARGET"
	SourceImported EvidenceSource = "IMPORTED"
)

func EvidenceSourceValues() []string { return []string{string(SourceTarget), string(SourceImported)} }

type Order string

const (
	OrderFinalizedDesc Order = "FINALIZED_DESC"
	OrderAcquiredDesc  Order = "ACQUIRED_DESC"
	OrderImportedDesc  Order = "IMPORTED_DESC"
)

func OrderValues() []string {
	return []string{string(OrderFinalizedDesc), string(OrderAcquiredDesc), string(OrderImportedDesc)}
}

type Query struct {
	PageSize      int
	Continuation  string
	Sources       []EvidenceSource
	Outcomes      []string
	EntrySkill    string
	SessionID     string
	FinalizedFrom *time.Time
	FinalizedTo   *time.Time
	AcquiredFrom  *time.Time
	AcquiredTo    *time.Time
	ImportedFrom  *time.Time
	ImportedTo    *time.Time
	Order         Order
	// Admit is an internal server-owned complete-item admission policy. It is
	// deliberately excluded from continuation fingerprints.
	Admit func(Entry) bool `json:"-"`
}

type LimitationCode string

const (
	LimitationTraceDiscoveryIncomplete      LimitationCode = "TRACE_DISCOVERY_INCOMPLETE"
	LimitationImportedEntrySkillUnavailable LimitationCode = "IMPORTED_ENTRY_SKILL_UNAVAILABLE"
	LimitationAmbiguousMetadataUnavailable  LimitationCode = "AMBIGUOUS_METADATA_UNAVAILABLE"
)

type Limitation struct {
	Code    LimitationCode
	Message string
}

type Entry struct {
	TraceID         string
	EvidenceSources []EvidenceSource
	SessionID       *string
	EntrySkill      *string
	Outcome         *string
	FinalizedAt     *time.Time
	AcquiredAt      *time.Time
	ImportedAt      *time.Time
	Ambiguous       bool
}

type Result struct {
	ObservedAt   time.Time
	Items        []Entry
	Complete     bool
	Limitations  []Limitation
	HasMore      bool
	Continuation string
}
