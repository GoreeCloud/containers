package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
)

type InventoryEntry struct {
	ID               string `json:"id"`
	MetadataPresent  bool   `json:"metadataPresent"`
	PendingOperation string `json:"pendingOperation,omitempty"`
	Status           string `json:"status"`
}
type InventoryReport struct {
	ReadOnly         bool             `json:"readOnly"`
	Entries          []InventoryEntry `json:"entries"`
	Running          int              `json:"running"`
	Created          int              `json:"created"`
	Stopped          int              `json:"stopped"`
	Uncertain        int              `json:"uncertain"`
	RecoveryRequired bool             `json:"recoveryRequired"`
	AtomicSnapshot   bool             `json:"atomicSnapshot"`
}

const maxInventory = 128

type inventoryIdentity struct {
	ID                string
	Metadata, Pending os.FileInfo
}

func scanInventory(root *os.Root) ([]inventoryIdentity, error) {
	list := map[string]inventoryIdentity{}
	for _, dir := range []string{".", "journal"} {
		f, e := root.Open(dir)
		if e != nil {
			return nil, e
		}
		names, e := f.Readdirnames(maxInventory + 3)
		f.Close()
		if e != nil && e != io.EOF {
			return nil, e
		}
		if len(names) > maxInventory+2 {
			return nil, errors.New("inventory bound exceeded")
		}
		for _, name := range names {
			if dir == "." && (name == "locks" || name == "journal") {
				continue
			}
			id := name
			if dir == "journal" {
				if !strings.HasSuffix(name, ".pending") {
					return nil, errors.New("unknown journal record")
				}
				id = strings.TrimSuffix(name, ".pending")
			}
			if ValidateID(id) != nil {
				return nil, errors.New("unexpected runtime inventory entry")
			}
			p := name
			if dir == "journal" {
				p = dir + "/" + name
			}
			info, e := root.Lstat(p)
			if e != nil {
				return nil, e
			}
			x := list[id]
			x.ID = id
			if dir == "." {
				if privateStateInfo(info) != nil {
					return nil, errors.New("unsafe runtime metadata directory")
				}
				x.Metadata = info
			} else {
				st, ok := info.Sys().(*syscall.Stat_t)
				if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
					return nil, errors.New("unsafe pending record")
				}
				x.Pending = info
			}
			list[id] = x
			if len(list) > maxInventory {
				return nil, errors.New("inventory ID limit exceeded")
			}
		}
	}
	out := make([]inventoryIdentity, 0, len(list))
	for _, x := range list {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func equalInventoryInfo(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	as, ok := a.Sys().(*syscall.Stat_t)
	bs, ok2 := b.Sys().(*syscall.Stat_t)
	return ok && ok2 && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && as.Ctim == bs.Ctim
}

// Inventory uses two anchored directory scans, and the existing cooperative
// read and recovery paths per ID. It is not an atomic snapshot or backend
// provenance claim. No raw runtime metadata, PID or paths are returned.
func (e *Engine) Inventory(ctx context.Context) (InventoryReport, error) {
	var empty InventoryReport
	if ctx == nil {
		return empty, errors.New("context required")
	}
	root, err := journalRoot(e.StateRoot)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	before, err := root.Stat(".")
	if err != nil {
		return empty, err
	}
	first, err := scanInventory(root)
	if err != nil {
		return empty, err
	}
	report := InventoryReport{ReadOnly: true, Entries: []InventoryEntry{}}
	for _, i := range first {
		if err = ctx.Err(); err != nil {
			return empty, err
		}
		row := InventoryEntry{ID: i.ID, MetadataPresent: i.Metadata != nil, Status: "uncertain"}
		if i.Pending != nil {
			observation, err := e.ObserveRecovery(ctx, i.ID)
			if err != nil || !observation.Pending.Pending {
				return empty, errors.New("pending inventory changed")
			}
			row.PendingOperation = observation.Pending.Operation
			report.RecoveryRequired = true
			if observation.Observed != nil {
				row.Status = observation.Observed.Status
			}
			report.Uncertain++
		} else {
			raw, err := e.State(ctx, i.ID)
			if err == nil {
				state, err := DecodeRuntimeState(raw, i.ID)
				if err == nil {
					row.Status = state.Status
					switch row.Status {
					case "running":
						report.Running++
					case "created":
						report.Created++
					case "stopped":
						report.Stopped++
					}
				}
			}
			if row.Status == "uncertain" {
				report.Uncertain++
				report.RecoveryRequired = true
			}
		}
		report.Entries = append(report.Entries, row)
	}
	second, err := scanInventory(root)
	if err != nil || len(first) != len(second) {
		return empty, errors.New("runtime inventory changed; retry")
	}
	for k, a := range first {
		b := second[k]
		if a.ID != b.ID || !equalInventoryInfo(a.Metadata, b.Metadata) || !equalInventoryInfo(a.Pending, b.Pending) {
			return empty, errors.New("runtime inventory changed; retry")
		}
	}
	current, err := os.Lstat(e.StateRoot)
	if err != nil || !os.SameFile(before, current) {
		return empty, errors.New("runtime root changed")
	}
	if err = ctx.Err(); err != nil {
		return empty, err
	}
	return report, nil
}
