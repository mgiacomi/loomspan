package traceanalysis

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Index component names. These are the derived bundle components written by the
// processor. They are closed logical identifiers, never paths.
const (
	ComponentRecordIndex   component = "records.idx"
	ComponentFrameIndex    component = "frames.idx"
	ComponentFrameDuration component = "frames-duration.idx"
	ComponentFrameUsage    component = "frames-usage.idx"
	ComponentAttemptIndex  component = "attempts.idx"
	ComponentRetryIndex    component = "retries.idx"
	ComponentValidationIdx component = "validations.idx"
	ComponentFailureIndex  component = "failures.idx"
	ComponentUsageIndex    component = "usage.idx"
	ComponentGapIndex      component = "gaps.idx"
	ComponentUncertainty   component = "uncertainties.idx"
	ComponentPayloadStore  component = "payloads.store"
	ComponentPayloadIndex  component = "payloads.idx"
	ComponentRecordFacts   component = "record-facts.store"
	ComponentRecordFactIdx component = "record-facts.idx"
)

// component is the logical name of one derived index/store component.
type component string

// recordIndexRow is the fixed-width entry in the record-address index: the
// canonical sequence and the raw byte offset/length for O(1)/binary-search
// lookup. Width is fixed so any row can be read by offset without scanning.
const recordIndexRowWidth = 32 // 8 (sequence) + 8 (offset) + 8 (length) + 8 (terminator)

// recordFactIndexRowWidth is one offset/length pair into record-facts.store.
// Rows align one-for-one with records.idx, so a selected record page resolves
// its facts with one fixed-width seek instead of rescanning every fact family.
const recordFactIndexRowWidth = 16

type recordFactIndexRow struct {
	Offset int64
	Length int64
}

func writeRecordFactIndexRow(w io.Writer, row recordFactIndexRow) error {
	var buf [recordFactIndexRowWidth]byte
	binary.LittleEndian.PutUint64(buf[0:8], uint64(row.Offset))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(row.Length))
	n, err := w.Write(buf[:])
	if err != nil {
		return err
	}
	if n != len(buf) {
		return errShortWrite
	}
	return nil
}

func readRecordFactIndexRow(buf []byte) recordFactIndexRow {
	return recordFactIndexRow{
		Offset: int64(binary.LittleEndian.Uint64(buf[0:8])),
		Length: int64(binary.LittleEndian.Uint64(buf[8:16])),
	}
}

// writeRecordIndexRow writes one fixed-width record-address row. It rejects a
// short write (fewer bytes written than requested) so a partial row is never
// published.
func writeRecordIndexRow(w io.Writer, row recordIndexRow) error {
	var buf [recordIndexRowWidth]byte
	binary.LittleEndian.PutUint64(buf[0:8], uint64(row.Sequence))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(row.Offset))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(row.Length))
	binary.LittleEndian.PutUint64(buf[24:32], uint64(row.TerminatorLength))
	n, err := w.Write(buf[:])
	if err != nil {
		return err
	}
	if n != recordIndexRowWidth {
		return errShortWrite
	}
	return nil
}

// recordIndexRow is one entry in the record-address index.
type recordIndexRow struct {
	Sequence         int64
	Offset           int64
	Length           int64
	TerminatorLength int64
}

// readRecordIndexRow reads one fixed-width record-address row from buf (exactly
// recordIndexRowWidth bytes).
func readRecordIndexRow(buf []byte) recordIndexRow {
	return recordIndexRow{
		Sequence:         int64(binary.LittleEndian.Uint64(buf[0:8])),
		Offset:           int64(binary.LittleEndian.Uint64(buf[8:16])),
		Length:           int64(binary.LittleEndian.Uint64(buf[16:24])),
		TerminatorLength: int64(binary.LittleEndian.Uint64(buf[24:32])),
	}
}

// writeLengthPrefixed writes a length-prefixed byte slice (uint32 little-endian
// length followed by bytes). Typed fact indexes use this framing. It rejects a
// short write so a partially framed row is never published.
func writeLengthPrefixed(w io.Writer, data []byte) error {
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if n, err := w.Write(lenBuf[:]); err != nil {
		return err
	} else if n != len(lenBuf) {
		return errShortWrite
	}
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return errShortWrite
	}
	return nil
}

// maxFactRowBytes is the maximum allowed size for a single length-prefixed fact
// row. No fact row should exceed one physical record's size (maxPhysicalLineBytes).
// A corrupt index with a larger declared length is rejected as a storage failure
// rather than triggering an unbounded allocation that could crash the console.
const maxFactRowBytes = maxPhysicalLineBytes

// readLengthPrefixed reads a length-prefixed value from r. It rejects a declared
// length exceeding maxFactRowBytes so a corrupt index component cannot trigger an
// unbounded allocation.
func readLengthPrefixed(r io.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(lenBuf[:])
	if n > maxFactRowBytes {
		return nil, fmt.Errorf("fact row length %d exceeds maximum %d", n, maxFactRowBytes)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
