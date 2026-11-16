package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"sort"
	"strings"
)

// JournalCensus is a redacted aggregate of unresolved OCI operations.
// The report never contains container IDs, bundle paths or process metadata.
type JournalCensus struct {
	ReadOnly             bool `json:"readOnly"`
	PendingCount         int  `json:"pendingCount"`
	PendingCreate        int  `json:"pendingCreate"`
	PendingStart         int  `json:"pendingStart"`
	PendingStop          int  `json:"pendingStop"`
	PendingDelete        int  `json:"pendingDelete"`
	RecoveryRequired     bool `json:"recoveryRequired"`
	RuntimeStateVerified bool `json:"runtimeStateVerified"`
}

const maxJournalCensusRecords = 1024

// InspectJournal reads an existing private directory, validates every
// committed pending marker through the strict per-container reader, and
// rejects unknown files/large inventories. It neither changes the runtime
// state nor infers whether any external process completed.
func InspectJournal(ctx context.Context, stateRoot string) (JournalCensus, error) {
	empty := JournalCensus{}
	if ctx == nil {
		return empty, errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	// Compare two consecutive bounded scans to avoid falsely declaring a
	// consistent census if cooperating changes occur while it is collected.
	first, firstID, err := inspectJournalOnce(ctx, stateRoot)
	if err != nil {
		return empty, err
	}
	second, secondID, err := inspectJournalOnce(ctx, stateRoot)
	if err != nil {
		return empty, err
	}
	if firstID != secondID || first != second {
		return empty, errors.New("OCI journal changed during inspection; retry read-only census")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return second, nil
}

// A privacy-preserving internal checksum detects changed entry identities and
// operation types across two passes. It MUST NOT be sent to the CLI caller:
// low-entropy container names are potentially recoverable from such a digest.
func inspectJournalOnce(ctx context.Context, stateRoot string) (JournalCensus, [32]byte, error) {
	empty := JournalCensus{}
	var zero [32]byte
	root, err := journalRoot(stateRoot)
	if err != nil {
		return empty, zero, err
	}
	defer root.Close()
	dir, err := root.Open("journal")
	if err != nil {
		return empty, zero, err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(maxJournalCensusRecords + 1)
	if err != nil && err != io.EOF {
		return empty, zero, err
	}
	if len(names) > maxJournalCensusRecords {
		return empty, zero, errors.New("OCI journal census entry limit exceeded")
	}
	sort.Strings(names)
	report := JournalCensus{ReadOnly: true, RuntimeStateVerified: false}
	checksum := sha256.New()
	for _, filename := range names {
		if err := ctx.Err(); err != nil {
			return empty, zero, err
		}
		if !strings.HasSuffix(filename, ".pending") {
			return empty, zero, errors.New("unknown OCI journal entry")
		}
		id := strings.TrimSuffix(filename, ".pending")
		if err := ValidateID(id); err != nil {
			return empty, zero, errors.New("invalid OCI journal entry name")
		}
		entry, err := ReadPendingMutation(ctx, stateRoot, id)
		if err != nil {
			return empty, zero, err
		}
		if !entry.Pending || !entry.RecoveryRequired {
			return empty, zero, errors.New("OCI journal changed during census")
		}
		switch entry.Operation {
		case "create":
			report.PendingCreate++
		case "start":
			report.PendingStart++
		case "kill":
			report.PendingStop++
		case "delete":
			report.PendingDelete++
		default:
			return empty, zero, errors.New("unknown OCI journal operation")
		}
		// Length-prefixed disambiguation via a NUL delimiter is safe because
		// strict OCI IDs and operation identifiers cannot contain NUL.
		checksum.Write([]byte(filename))
		checksum.Write([]byte{0})
		checksum.Write([]byte(entry.Operation))
		checksum.Write([]byte{0})
		report.PendingCount++
	}
	report.RecoveryRequired = report.PendingCount > 0
	if err := ctx.Err(); err != nil {
		return empty, zero, err
	}
	var identity [32]byte
	copy(identity[:], checksum.Sum(nil))
	return report, identity, nil
}
