package traceanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/artifact"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/consolecore"
)

// indexWriter owns streaming writes to every derived index/store component. It
// validates each write and final size; a partially written component is never
// published because the processor returns an error and the service removes the
// staged bundle.
type indexWriter struct {
	sink            artifact.ComponentSink
	ctx             context.Context
	scopeID         string
	components      map[component]int64
	recordWriter    artifact.ComponentWriter
	recordCount     int64
	recordSize      int64
	recordSequences []int64
}

// writeRecordFacts writes one fixed-width address row per canonical record and
// stores only non-empty fact bodies. The address table is aligned with
// records.idx, making page enrichment proportional to returned records.
func (w *indexWriter) writeRecordFacts(facts map[int64]persistedRecordFacts) *consolecore.Error {
	store, domain := w.create(ComponentRecordFacts)
	if domain != nil {
		return domain
	}
	index, domain := w.create(ComponentRecordFactIdx)
	if domain != nil {
		_ = store.Close()
		return domain
	}
	var storeSize, indexSize int64
	for _, sequence := range w.recordSequences {
		row := recordFactIndexRow{Offset: storeSize}
		if value, ok := facts[sequence]; ok && !value.empty() {
			body, err := json.Marshal(value)
			if err != nil {
				_ = index.Close()
				return w.failClose(store, err)
			}
			if len(body) > maxFactRowBytes {
				_ = index.Close()
				return w.failClose(store, fmt.Errorf("record fact row length %d exceeds maximum %d", len(body), maxFactRowBytes))
			}
			if n, err := store.Write(body); err != nil || n != len(body) {
				_ = index.Close()
				if err == nil {
					err = errShortWrite
				}
				return w.failClose(store, err)
			}
			row.Length = int64(len(body))
			storeSize += row.Length
		}
		if err := writeRecordFactIndexRow(index, row); err != nil {
			_ = store.Close()
			return w.failClose(index, err)
		}
		indexSize += recordFactIndexRowWidth
	}
	if err := w.syncClose(store); err != nil {
		_ = index.Close()
		return err
	}
	if err := w.syncClose(index); err != nil {
		return err
	}
	w.components[ComponentRecordFacts] = storeSize
	w.components[ComponentRecordFactIdx] = indexSize
	return nil
}

// newIndexWriter creates an index writer bound to a sink and processing context.
func newIndexWriter(sink artifact.ComponentSink, ctx context.Context, scopeID string) *indexWriter {
	return &indexWriter{
		sink:       sink,
		ctx:        ctx,
		scopeID:    scopeID,
		components: map[component]int64{},
	}
}

// create opens a derived component for streaming writes.
func (w *indexWriter) create(name component) (artifact.ComponentWriter, *consolecore.Error) {
	if e := w.ctx.Err(); e != nil {
		return nil, canceledError(e)
	}
	writer, domain := w.sink.Create(w.ctx, artifact.ComponentName(name))
	if domain != nil {
		return nil, domain
	}
	return writer, nil
}

func (w *indexWriter) startRecordIndex() *consolecore.Error {
	writer, domain := w.create(ComponentRecordIndex)
	if domain != nil {
		return domain
	}
	w.recordWriter = writer
	return nil
}

// appendRecordRow streams one canonical address row directly to disk.
func (w *indexWriter) appendRecordRow(row recordIndexRow) *consolecore.Error {
	if w.recordWriter == nil {
		return w.storageError(errors.New("record index is not open"))
	}
	if err := writeRecordIndexRow(w.recordWriter, row); err != nil {
		return w.storageError(err)
	}
	w.recordCount++
	w.recordSize += recordIndexRowWidth
	w.recordSequences = append(w.recordSequences, row.Sequence)
	return nil
}

func (w *indexWriter) flushRecordIndex() *consolecore.Error {
	if w.recordWriter == nil {
		return w.storageError(errors.New("record index is not open"))
	}
	writer := w.recordWriter
	w.recordWriter = nil
	if err := w.syncClose(writer); err != nil {
		return err
	}
	w.components[ComponentRecordIndex] = w.recordSize
	return nil
}

func (w *indexWriter) abortRecordIndex() {
	if w.recordWriter != nil {
		_ = w.recordWriter.Close()
		w.recordWriter = nil
	}
}

// writePayloadStore records the reconstructed payload store size. The assembler
// streams bytes to the store writer during parsing; this records the final size
// for the manifest and component accounting.
func (w *indexWriter) recordPayloadStoreSize(size int64) {
	w.components[ComponentPayloadStore] = size
}

// syncClose syncs and closes a component writer, mapping failures to a storage
// domain error.
func (w *indexWriter) syncClose(writer artifact.ComponentWriter) *consolecore.Error {
	if err := writer.Sync(); err != nil {
		_ = writer.Close()
		return w.storageError(err)
	}
	if err := writer.Close(); err != nil {
		return w.storageError(err)
	}
	return nil
}

// failClose closes a writer after a write failure and returns the storage error.
func (w *indexWriter) failClose(writer artifact.ComponentWriter, cause error) *consolecore.Error {
	_ = writer.Close()
	return w.storageError(cause)
}

// storageError maps a storage failure to a domain error.
func (w *indexWriter) storageError(cause error) *consolecore.Error {
	return consolecore.NewError(consolecore.CodeLocalStorageUnavailable,
		"Local artifact storage is unavailable.", w.scopeID, consolecore.Details{}, cause)
}

// writeFactRows writes length-prefixed JSON rows directly to a component. Each
// fact is marshaled independently and framed with a uint32 length so the index
// can be streamed back row-by-row without materializing the whole list.
func (w *indexWriter) writeFactRows(name component, facts []any) *consolecore.Error {
	writer, domain := w.create(name)
	if domain != nil {
		return domain
	}
	var size int64
	for _, fact := range facts {
		row, err := json.Marshal(fact)
		if err != nil {
			return w.failClose(writer, err)
		}
		if err := writeLengthPrefixed(writer, row); err != nil {
			return w.failClose(writer, err)
		}
		size += int64(4 + len(row))
	}
	if err := w.syncClose(writer); err != nil {
		return err
	}
	w.components[name] = size
	return nil
}
