package stack

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"syscall"
)

var mappingName = regexp.MustCompile("^lease-[a-f0-9]{32}\\.json$")
var errMappingInUse = errors.New("subordinate identity already reserved in shared store")

type mappingRecord struct {
	Version int      `json:"version"`
	Session string   `json:"session"`
	UIDs    []uint32 `json:"uids"`
	GIDs    []uint32 `json:"gids"`
}
type mappingReference struct {
	Store  string `json:"store"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

func validateMappingRecord(r mappingRecord) error {
	if r.Version != 1 || !filepath.IsAbs(r.Session) || filepath.Clean(r.Session) != r.Session || !localSessionName.MatchString(filepath.Base(r.Session)) || len(r.UIDs) < 1 || len(r.UIDs) > 4 || len(r.UIDs) != len(r.GIDs) {
		return errors.New("invalid mapping reservation")
	}
	for _, ids := range [][]uint32{r.UIDs, r.GIDs} {
		for i, v := range ids {
			if v < 65536 || v == ^uint32(0) || i > 0 && ids[i-1] >= v {
				return errors.New("invalid mapping reservation identity")
			}
		}
	}
	return nil
}
func parseMappingRecord(raw []byte) (mappingRecord, error) {
	var r mappingRecord
	if e := rejectDuplicate(raw); e != nil {
		return r, e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&r); e != nil {
		return r, e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return r, errors.New("trailing mapping reservation")
	}
	return r, validateMappingRecord(r)
}
func scanMappingRecords(l *localLease) (map[string]mappingRecord, error) {
	if e := l.check(); e != nil {
		return nil, e
	}
	f, e := l.root.Open(".")
	if e != nil {
		return nil, e
	}
	names, e := f.Readdirnames(66)
	f.Close()
	if e != nil && e != io.EOF {
		return nil, e
	}
	if len(names) > 65 {
		return nil, errors.New("mapping reservation quota exceeded")
	}
	records := map[string]mappingRecord{}
	u, g := map[uint32]bool{}, map[uint32]bool{}
	for _, name := range names {
		if name == "local-session.lock" {
			continue
		}
		if !mappingName.MatchString(name) {
			return nil, errors.New("unexpected shared mapping store entry")
		}
		raw, _, e := readPrivateRecord(l.root, name, 4096)
		if e != nil {
			return nil, e
		}
		r, e := parseMappingRecord(raw)
		if e != nil {
			return nil, e
		}
		for _, v := range r.UIDs {
			if u[v] {
				return nil, errors.New("conflicting existing UID reservations")
			}
			u[v] = true
		}
		for _, v := range r.GIDs {
			if g[v] {
				return nil, errors.New("conflicting existing GID reservations")
			}
			g[v] = true
		}
		records[name] = r
	}
	return records, l.check()
}
func reserveMappings(l *localLease, session string, uids, gids map[uint32]bool) (mappingReference, error) {
	var ref mappingReference
	r := mappingRecord{Version: 1, Session: session}
	for u := range uids {
		r.UIDs = append(r.UIDs, u)
	}
	for g := range gids {
		r.GIDs = append(r.GIDs, g)
	}
	sort.Slice(r.UIDs, func(i, j int) bool { return r.UIDs[i] < r.UIDs[j] })
	sort.Slice(r.GIDs, func(i, j int) bool { return r.GIDs[i] < r.GIDs[j] })
	if e := validateMappingRecord(r); e != nil {
		return ref, e
	}
	records, e := scanMappingRecords(l)
	if e != nil {
		return ref, e
	}
	if len(records) >= 64 {
		return ref, errors.New("shared mapping store full")
	}
	for _, old := range records {
		if old.Session == session {
			return ref, errors.New("session reservation already exists")
		}
		for _, u := range old.UIDs {
			if uids[u] {
				return ref, errMappingInUse
			}
		}
		for _, g := range old.GIDs {
			if gids[g] {
				return ref, errMappingInUse
			}
		}
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return ref, e
	}
	name := fmt.Sprintf("lease-%x.json", nonce)
	raw, e := json.Marshal(r)
	if e != nil {
		return ref, e
	}
	f, e := l.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return ref, e
	}
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	if e = errors.Join(e, f.Close()); e != nil {
		return ref, e
	}
	if e = syncDirectory(l.root); e != nil {
		return ref, e
	}
	ref = mappingReference{Store: l.parent, Name: name, SHA256: digestBytes(raw)}
	if e = checkMapping(l, ref, session); e != nil {
		return ref, e
	}
	return ref, nil
}
func validMappingReference(ref mappingReference) bool {
	return filepath.IsAbs(ref.Store) && filepath.Clean(ref.Store) == ref.Store && mappingName.MatchString(ref.Name) && imagePattern.MatchString(ref.SHA256)
}
func checkMapping(l *localLease, ref mappingReference, session string) error {
	if !validMappingReference(ref) || l.parent != ref.Store {
		return errors.New("invalid shared mapping reference")
	}
	records, e := scanMappingRecords(l)
	if e != nil {
		return e
	}
	r, ok := records[ref.Name]
	if !ok || r.Session != session {
		return errors.New("shared mapping reservation absent or changed")
	}
	raw, _, e := readPrivateRecord(l.root, ref.Name, 4096)
	if e != nil || digestBytes(raw) != ref.SHA256 {
		return errors.New("mapping reservation pin mismatch")
	}
	return nil
}

// Internal only, after verified runtime cleanup. No CLI force-release.
func releaseMappings(l *localLease, ref mappingReference, session string) error {
	if e := checkMapping(l, ref, session); e != nil {
		return e
	}
	if e := l.root.Remove(ref.Name); e != nil {
		return e
	}
	return syncDirectory(l.root)
}
func acquireMappingStore(ctx context.Context, store, parent string) (*localLease, error) {
	if store == parent {
		return nil, errors.New("shared mapping store must be distinct from session parent")
	}
	// Never make the session parent a descendant of the lease store or vice versa.
	for _, pair := range [][2]string{{store, parent}, {parent, store}} {
		if rel, e := filepath.Rel(pair[0], pair[1]); e == nil && filepath.IsLocal(rel) {
			return nil, errors.New("mapping store and session parent must be separate")
		}
	}
	return acquireLocalLease(ctx, store)
}
