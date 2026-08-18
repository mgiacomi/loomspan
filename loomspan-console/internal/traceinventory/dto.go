package traceinventory

import "time"

type Query struct {
	PageSize     int
	Continuation string
}

type LimitationCode string

const LimitationTraceDiscoveryIncomplete LimitationCode = "TRACE_DISCOVERY_INCOMPLETE"

type Limitation struct {
	Code    LimitationCode
	Message string
}

type Entry struct {
	TraceID     string
	SessionID   string
	EntrySkill  string
	Outcome     string
	FinalizedAt time.Time
	Ambiguous   bool
}

type Result struct {
	ObservedAt   time.Time
	Items        []Entry
	Complete     bool
	Limitations  []Limitation
	HasMore      bool
	Continuation string
}
