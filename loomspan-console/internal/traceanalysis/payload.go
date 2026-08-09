package traceanalysis

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"unicode/utf8"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// A canonical error may contain four MiB of decoded diagnostic text. JSON can
// require six ASCII bytes for one decoded control byte (for example \u0000), so
// retain a bounded serialized representation large enough for the canonical
// decoded contract plus descriptor and contextual-field overhead.
const maxFailureLogicalPayloadBytes = 25 << 20

// payloadDescriptor records one reconstructed logical payload. StoreOffset and
// StoreLength locate the reconstructed bytes inside the payload store component.
// They are persisted in the payload index via payloadIndexRow so range queries
// can seek directly, but are excluded from the semantic comparison shape.
type payloadDescriptor struct {
	PayloadID   string `json:"payloadId"`
	Sequence    int64  `json:"logicalRecordSequence"`
	ContentType string `json:"contentType"`
	ChunkCount  int    `json:"chunkCount"`
	StoreOffset int64  `json:"-"`
	StoreLength int64  `json:"-"`
}

// payloadIndexRow is the persisted shape of a payload descriptor in the payload
// index. It includes StoreOffset and StoreLength so range queries can seek
// directly into the payload store component.
type payloadIndexRow struct {
	PayloadID   string `json:"payloadId"`
	Sequence    int64  `json:"logicalRecordSequence"`
	ContentType string `json:"contentType"`
	ChunkCount  int    `json:"chunkCount"`
	StoreOffset int64  `json:"storeOffset"`
	StoreLength int64  `json:"storeLength"`
}

// payloadBuild tracks one in-flight chunked payload during streaming assembly.
type payloadBuild struct {
	descriptor     payloadDescriptor
	nextIndex      int
	received       int
	storeOffset    int64
	storeLength    int64
	validator      reconstructedValidator
	logical        bytes.Buffer
	captureLogical bool
}

// payloadAssembler validates chunk contracts, streams reconstructed content to a
// payload store, and validates reconstructed JSON/UTF-8 without materializing
// the full logical value. One assembler instance handles one processing run.
type payloadAssembler struct {
	store        io.Writer
	builds       map[string]*payloadBuild
	descriptors  []payloadDescriptor
	storeWritten int64
	activeID     string
}

// newPayloadAssembler creates an assembler that streams reconstructed payloads to
// store. The store is the payloads.store bundle component writer.
func newPayloadAssembler(store io.Writer) *payloadAssembler {
	return &payloadAssembler{store: store, builds: map[string]*payloadBuild{}}
}

// onEnvelope registers a chunked-payload envelope record.
func (a *payloadAssembler) onEnvelope(rec *Record) *consolecore.Error {
	if a.activeID != "" {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	count, ok := envelopeChunkCount(rec)
	if !ok || count <= 0 {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	contentType := rec.metadataStringOrEmpty("contentType")
	if contentType == "" {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	if _, dup := a.builds[rec.PayloadID]; dup {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	build := &payloadBuild{
		descriptor: payloadDescriptor{
			PayloadID:   rec.PayloadID,
			Sequence:    rec.Sequence,
			ContentType: contentType,
			ChunkCount:  count,
		},
		nextIndex:      0,
		storeOffset:    a.storeWritten,
		validator:      newReconstructedValidator(contentType),
		captureLogical: rec.Type == RecordErrorRecorded,
	}
	a.builds[rec.PayloadID] = build
	a.activeID = rec.PayloadID
	return nil
}

// onChunk validates and appends one chunk record's content to the payload store.
func (a *payloadAssembler) onChunk(rec *Record) *consolecore.Error {
	payloadID, ok := chunkPayloadID(rec)
	if !ok {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	build, exists := a.builds[payloadID]
	if !exists || a.activeID != payloadID {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	chunkIndex, indexPresent, indexValid := rec.metadataIntStrict("chunkIndex")
	chunkCount, countPresent, countValid := rec.metadataIntStrict("chunkCount")
	if !indexPresent || !indexValid || !countPresent || !countValid {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	if int(chunkCount) != build.descriptor.ChunkCount {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	contentType := rec.metadataStringOrEmpty("contentType")
	if contentType != build.descriptor.ContentType {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	if int(chunkIndex) != build.nextIndex {
		// Duplicate, out-of-order, or skipped index.
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	content, err := decodeChunkContent(rec)
	if err != nil {
		return invalidityError(CategoryInvalidChunks, rec.TraceID)
	}
	if build.validator != nil {
		if err := build.validator.write(content); err != nil {
			return invalidityError(CategoryInvalidChunks, rec.TraceID)
		}
	}
	if build.captureLogical {
		if build.logical.Len()+len(content) > maxFailureLogicalPayloadBytes {
			return invalidityError(CategoryInvalidChunks, rec.TraceID)
		}
		_, _ = build.logical.Write(content)
	}
	n, err := a.store.Write(content)
	if err != nil {
		return consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", rec.TraceID, consolecore.Details{}, err)
	}
	if n != len(content) {
		return consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
			"Local artifact storage is unavailable.", rec.TraceID, consolecore.Details{}, errShortWrite)
	}
	build.nextIndex++
	build.received++
	build.storeLength += int64(n)
	a.storeWritten += int64(n)
	if build.received == build.descriptor.ChunkCount {
		a.activeID = ""
	}
	return nil
}

func (a *payloadAssembler) logicalPayload(payloadID string) []byte {
	if build := a.builds[payloadID]; build != nil {
		return build.logical.Bytes()
	}
	return nil
}

// finalize verifies every envelope received its complete chunk set and closes
// reconstructed-content validators. It records the final descriptors.
func (a *payloadAssembler) finalize() *consolecore.Error {
	for _, build := range a.builds {
		if build.received != build.descriptor.ChunkCount {
			return invalidityError(CategoryIncompleteChunks, build.descriptor.PayloadID)
		}
		if build.validator != nil {
			validator := build.validator
			build.validator = nil
			if err := validator.close(); err != nil {
				return invalidityError(CategoryInvalidChunks, build.descriptor.PayloadID)
			}
		}
		build.descriptor.StoreOffset = build.storeOffset
		build.descriptor.StoreLength = build.storeLength
		a.descriptors = append(a.descriptors, build.descriptor)
	}
	return nil
}

// cleanup releases any in-flight reconstructed-content validators without
// finalizing descriptors. It must be called on every error path where
// finalize is not reached so the per-build validator goroutines (which block
// on a pipe reader) are unblocked and do not leak.
func (a *payloadAssembler) cleanup() {
	for _, build := range a.builds {
		if build.validator != nil {
			validator := build.validator
			build.validator = nil
			_ = validator.close()
		}
	}
}

// descriptorsInOrder returns payload descriptors in canonical (envelope
// sequence) order.
func (a *payloadAssembler) descriptorsInOrder() []payloadDescriptor {
	out := make([]payloadDescriptor, len(a.descriptors))
	copy(out, a.descriptors)
	// Stable sort by sequence; payloads are appended in arrival order which is
	// already canonical for the current writer, but sort to be deterministic.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}

// decodeChunkContent extracts the raw string content of a chunk record's data
// field. The data is a JSON string; its decoded bytes are the chunk content.
func decodeChunkContent(rec *Record) ([]byte, error) {
	if len(rec.Data) == 0 {
		return nil, errors.New("chunk record has no data")
	}
	var s string
	if err := json.Unmarshal(rec.Data, &s); err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// envelopeChunkCount extracts the declared chunk count from an envelope record.
func envelopeChunkCount(rec *Record) (int, bool) {
	v, present, valid := rec.metadataIntStrict("chunkCount")
	if !present || !valid {
		return 0, false
	}
	maxInt := int64(^uint(0) >> 1)
	if v <= 0 || v > maxInt {
		return 0, false
	}
	return int(v), true
}

// chunkPayloadID extracts the payloadId from a chunk record's metadata.
func chunkPayloadID(rec *Record) (string, bool) {
	id := rec.metadataStringOrEmpty("payloadId")
	if id == "" {
		return "", false
	}
	return id, true
}

// errShortWrite is returned when a store write does not consume all bytes.
var errShortWrite = errors.New("short write to payload store")

// reconstructedValidator validates reconstructed payload content incrementally
// as chunks arrive, without materializing the full logical value.
type reconstructedValidator interface {
	write(p []byte) error
	close() error
}

// newReconstructedValidator selects a streaming validator by content type.
func newReconstructedValidator(contentType string) reconstructedValidator {
	switch contentType {
	case "application/json":
		return newJSONStreamValidator()
	case "text/plain":
		return &utf8StreamValidator{}
	default:
		// Unknown content types are not validated; the chunk contract still
		// guarantees count/index/order. This keeps unconsumed content opaque.
		return nil
	}
}

// jsonStreamValidator validates that the reconstructed bytes form exactly one
// complete JSON value, streaming through an io.Pipe so the full value is never
// materialized in memory.
type jsonStreamValidator struct {
	pw       *io.PipeWriter
	done     chan jsonResult
	depth    int
	inString bool
	escape   bool
}
type jsonResult struct {
	err error
}

func newJSONStreamValidator() *jsonStreamValidator {
	pr, pw := io.Pipe()
	v := &jsonStreamValidator{pw: pw, done: make(chan jsonResult, 1)}
	go func() {
		dec := json.NewDecoder(pr)
		err := dec.Decode(&json.RawMessage{})
		if err == nil {
			var trailing json.RawMessage
			if trailingErr := dec.Decode(&trailing); trailingErr != io.EOF {
				if trailingErr == nil {
					err = errors.New("reconstructed payload has trailing content")
				} else {
					err = trailingErr
				}
			}
		}
		v.done <- jsonResult{err: err}
		_ = pr.Close()
	}()
	return v
}

func (v *jsonStreamValidator) write(p []byte) error {
	for _, b := range p {
		if v.escape {
			v.escape = false
			continue
		}
		if v.inString {
			if b == '\\' {
				v.escape = true
			} else if b == '"' {
				v.inString = false
			}
			continue
		}
		switch b {
		case '"':
			v.inString = true
		case '{', '[':
			v.depth++
			if v.depth > maxJSONDepth {
				return errors.New("reconstructed payload exceeds JSON depth limit")
			}
		case '}', ']':
			v.depth--
			if v.depth < 0 {
				return errors.New("reconstructed payload has unbalanced JSON structure")
			}
		}
	}
	_, err := v.pw.Write(p)
	return err
}

func (v *jsonStreamValidator) close() error {
	_ = v.pw.Close()
	r := <-v.done
	if r.err != nil {
		return r.err
	}
	return nil
}

// utf8StreamValidator validates UTF-8 across chunk boundaries by retaining at
// most three incomplete leading bytes between writes.
type utf8StreamValidator struct {
	pending []byte
}

func (v *utf8StreamValidator) write(p []byte) error {
	buf := p
	if len(v.pending) > 0 {
		buf = append(append([]byte(nil), v.pending...), p...)
		v.pending = v.pending[:0]
	}
	valid, rest := utf8ValidPrefix(buf)
	if !valid {
		return errors.New("reconstructed payload is not valid UTF-8")
	}
	if len(rest) > 0 {
		v.pending = append(v.pending[:0], rest...)
	}
	return nil
}

func (v *utf8StreamValidator) close() error {
	if len(v.pending) > 0 {
		return errors.New("reconstructed payload ends inside a UTF-8 rune")
	}
	return nil
}

// utf8ValidPrefix reports whether buf is valid UTF-8 up to a possible incomplete
// trailing rune. It returns the trailing incomplete bytes (0-3) when valid.
func utf8ValidPrefix(buf []byte) (bool, []byte) {
	for len(buf) > 0 {
		if !utf8.FullRune(buf) {
			return true, buf
		}
		r, size := utf8.DecodeRune(buf)
		if r == utf8.RuneError && size == 1 {
			return false, nil
		}
		buf = buf[size:]
	}
	return true, nil
}
