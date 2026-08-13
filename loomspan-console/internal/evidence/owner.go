package evidence

import (
	"fmt"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/target"
)

// Source identifies the authority and lifecycle owner of installed evidence.
type Source string

const (
	SourceTarget   Source = "TARGET"
	SourceImported Source = "IMPORTED"
)

// Owner is a comparable, process-local artifact owner. Its ID is opaque and
// never a filesystem path. TargetScope is populated only for TARGET owners.
type Owner struct {
	source      Source
	id          string
	targetScope target.ScopeID
}

// Reference is the adapter-safe evidence selector. Imported references never
// expose the service-owned process-local owner ID.
type Reference struct {
	Source      Source
	TargetScope target.ScopeID
}

func ForTarget(scope target.ScopeID) Reference {
	return Reference{Source: SourceTarget, TargetScope: scope}
}

func ForImported() Reference { return Reference{Source: SourceImported} }

func (ref Reference) Valid() bool {
	return (ref.Source == SourceTarget && ref.TargetScope != "") ||
		(ref.Source == SourceImported && ref.TargetScope == "")
}

func (ref Reference) ID() string {
	if ref.Source == SourceTarget {
		return string(ref.TargetScope)
	}
	return string(ref.Source)
}

func Target(scope target.ScopeID) Owner {
	return Owner{source: SourceTarget, id: string(scope), targetScope: scope}
}

func Imported(id string) (Owner, error) {
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, `/\\:`) {
		return Owner{}, fmt.Errorf("imported evidence owner ID must be opaque and nonblank")
	}
	return Owner{source: SourceImported, id: id}, nil
}

func (owner Owner) Source() Source              { return owner.source }
func (owner Owner) ID() string                  { return owner.id }
func (owner Owner) TargetScope() target.ScopeID { return owner.targetScope }

func (owner Owner) Valid() bool {
	if owner.source == SourceTarget {
		return owner.id != "" && owner.targetScope != "" && owner.id == string(owner.targetScope)
	}
	return owner.source == SourceImported && owner.id != "" && owner.targetScope == ""
}
