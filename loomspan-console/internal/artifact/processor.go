package artifact

import (
	"context"
	"io"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// ComponentName is the logical name of one bundle component. The raw artifact is
// always ComponentRawArtifact; derived components (indexes, payload store,
// manifest) are named by the processor. Component names are closed logical
// identifiers, never caller-supplied filesystem paths: they must not contain
// path separators and must be non-empty.
type ComponentName string

const (
	// ComponentRawArtifact is the unchanged raw NDJSON artifact. It is always
	// present in a published bundle and is written by the acquisition leader
	// before the processor runs.
	ComponentRawArtifact ComponentName = "artifact.ndjson"
)

// ComponentWriter is a writable, syncable handle to one derived bundle
// component. The processor writes derived components through the sink; it never
// sees the absolute path. Each Write reserves capacity for the bytes about to
// be written before forwarding them to disk, so a capacity rejection never
// leaves a partial uncharged component.
type ComponentWriter interface {
	io.Writer
	io.Closer
	Sync() error
}

// ComponentReader is a seekable reader over one installed bundle component. It
// is issued by a Lease and is valid until the lease or the reader is closed.
type ComponentReader interface {
	io.ReadSeekCloser
}

// ComponentSink lets the processor create derived bundle components inside the
// staged bundle. The sink reserves capacity before each derived write, accounts
// short writes/sync/close failures, and reports only logical component names.
// The sink never exposes an absolute path.
type ComponentSink interface {
	// Create opens a new derived component for streaming writes. The name must
	// be a logical component identifier, never a path. Creating the same name
	// twice during one processing run replaces the in-progress component.
	Create(ctx context.Context, name ComponentName) (ComponentWriter, *consolecore.Error)
}

// ProcessRequest carries the inputs to one processor execution. The processor
// receives a cancellable context, immutable trace metadata, a reader over the
// already-installed raw artifact, and a sink for derived components. It never
// receives an absolute path.
type ProcessRequest struct {
	Context  context.Context
	Metadata TraceMetadata
	Raw      io.Reader
	Sink     ComponentSink
}

// ProcessResult carries the derived component sizes the processor produced. The
// raw artifact size is tracked separately by the service; only derived
// components are reported here.
type ProcessResult struct {
	// ComponentSizes maps each derived component name to its final synced byte
	// count.
	ComponentSizes map[ComponentName]int64
	// Metadata is derived from the fully validated canonical file. Callers must
	// not publish caller-supplied identity or completion facts in its place.
	Metadata TraceMetadata
}

// Processor validates a raw artifact and produces derived bundle components
// before the artifact handle is published. It is a required dependency: an
// artifact is admitted only after the processor succeeds. On any invalidity,
// cancellation, or recoverable storage failure, the processor returns a domain
// error and the service removes the entire staged bundle without publishing a
// handle.
type Processor interface {
	Process(req ProcessRequest) (ProcessResult, *consolecore.Error)
}

// ImportHeader is the bounded identity extracted from the canonical first
// record before an imported artifact reserves an owner-local identity.
type ImportHeader struct {
	TraceID   string
	SessionID string
}

// ImportPreflight is the validated header plus a byte-exact replay stream. The
// replay begins at byte zero and continues from the original reader.
type ImportPreflight struct {
	Header ImportHeader
	Raw    io.Reader
}

// ImportProcessor owns bounded canonical-header interpretation for imports.
// It is deliberately separate from the complete Process pass.
type ImportProcessor interface {
	Processor
	PreflightImport(context.Context, io.Reader) (ImportPreflight, *consolecore.Error)
}

// validateComponentName reports whether a component name is a safe logical
// identifier rather than a path. It rejects empty names, names containing path
// separators, and names that traverse the bundle directory.
func validateComponentName(name ComponentName) *consolecore.Error {
	s := string(name)
	if s == "" {
		return consolecore.NewError(consolecore.CodeConsoleError,
			"The artifact component name is empty.", "", consolecore.Details{}, nil)
	}
	if s == "." || s == ".." || containsPathSeparator(s) {
		return consolecore.NewError(consolecore.CodeConsoleError,
			"The artifact component name is invalid.", "", consolecore.Details{}, nil)
	}
	return nil
}

// containsPathSeparator reports whether s contains any character that would let
// it escape the bundle directory on any supported platform.
func containsPathSeparator(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '/' || c == '\\' || c == ':' {
			return true
		}
	}
	return false
}
