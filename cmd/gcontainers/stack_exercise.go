package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/GoreeCloud/containers/internal/stack"
	"io"
	"os/exec"
	"path/filepath"
	"time"
)

func stackExercise(args []string, out io.Writer, review bool) error {
	if review {
		if len(args) != 3 {
			return errors.New("usage: stack review-exercise SPEC ADMISSION ADMISSION_SHA")
		}
		s, e := stack.Load(args[0])
		if e != nil {
			return e
		}
		_, _, r, e := stack.LoadExercise(s, args[1], args[2])
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(r)
	}
	if len(args) != 6 || args[5] != "--experimental" {
		return errors.New("usage: stack exercise SPEC ADMISSION ADMISSION_SHA EXISTING_STORE PRIVATE_PARENT --experimental")
	}
	s, e := stack.Load(args[0])
	if e != nil {
		return e
	}
	bin, e := exec.LookPath("runc")
	if e != nil {
		return e
	}
	bin, e = filepath.Abs(bin)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	r, e := stack.RunExercise(ctx, s, args[1], args[2], args[3], args[4], bin)
	if write := json.NewEncoder(out).Encode(r); write != nil {
		return errors.Join(e, write)
	}
	return e
}
