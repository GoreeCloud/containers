package engine

import (
	"context"
	"errors"
	"io"
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
	var empty JournalCensus
	if ctx == nil {
		return empty, errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	root, err := journalRoot(stateRoot)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	dir, err := root.Open("journal")
	if err != nil {
		return empty, err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(maxJournalCensusRecords + 1)
	if err != nil && err != io.EOF {
		return empty, err
	}
	if len(names) > maxJournalCensusRecords {
		return empty, errors.New("OCI journal census entry limit exceeded")
	}
	report := JournalCensus{ReadOnly: true, RuntimeStateVerified: false}
	for _, filename := range names {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		if !strings.HasSuffix(filename, ".pending") {
			return empty, errors.New("unknown OCI journal entry")
		}
		id := strings.TrimSuffix(filename, ".pending")
		if err := ValidateID(id); err != nil {
			return empty, errors.New("invalid OCI journal entry name")
		}
		entry, err := ReadPendingMutation(ctx, stateRoot, id)
		if err != nil {
			return empty, err
		}
		if !entry.Pending || !entry.RecoveryRequired {
			return empty, errors.New("OCI journal entries changed during census")
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
			return empty, errors.New("unknown OCI journal operation")
		}
		report.PendingCount++
	}
	report.RecoveryRequired = report.PendingCount > 0
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	return report, nil
}
