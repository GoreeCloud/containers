package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/volume"
	"io"
	"time"
)

func volumeCommand(args []string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	switch {
	case (len(args) == 3 || len(args) == 4) && args[0] == "fingerprint":
		pin := ""
		if len(args) == 4 {
			pin = args[3]
			if pin == "" {
				return errors.New("empty digest")
			}
		}
		r, e := volume.Fingerprint(ctx, args[1], args[2], pin)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(r)
	case len(args) == 3 && args[0] == "verify-backup":
		r, e := volume.VerifyBackup(ctx, args[1], args[2])
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(r)

	case len(args) == 5 && args[0] == "backup" && args[4] == "--experimental":
		r, e := volume.Backup(ctx, args[1], args[2], args[3])
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(r)
	case len(args) == 6 && args[0] == "restore" && args[5] == "--experimental":
		r, e := volume.Restore(ctx, args[1], args[2], args[3], args[4])
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(r)

	case len(args) == 3 && args[0] == "inspect":
		r, e := volume.Inspect(ctx, args[1], args[2])
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(r)
	case len(args) == 4 && args[0] == "create" && args[3] == "--experimental":
		r, e := volume.Create(ctx, args[1], args[2])
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(r)
	case len(args) == 2 && args[0] == "count":
		n, e := volume.Count(ctx, args[1])
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(map[string]any{"count": n, "readOnly": true, "mountVerified": false})
	default:
		return errors.New("usage: volume create PRIVATE_STORE ID --experimental | inspect PRIVATE_STORE ID | count PRIVATE_STORE | verify-backup BACKUP_DIR SHA256 | backup/restore (see help)")
	}
}
