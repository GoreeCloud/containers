package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/GoreeCloud/containers/internal/stack"
)

func stackRevisionCheck(args []string, out io.Writer) error {
	if len(args) < 3 {
		return errors.New("usage: stack revision-check OLD_JSON NEW_JSON --expected-current-sha256 sha256:HEX")
	}
	flags := flag.NewFlagSet("stack revision-check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expected-current-sha256", "", "expected canonical source declaration revision")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected revision-check arguments")
	}
	before, err := stack.Load(args[0])
	if err != nil {
		return err
	}
	after, err := stack.Load(args[1])
	if err != nil {
		return err
	}
	review, err := stack.ReviewRevision(before, after, *expected)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintln(out, string(encoded)); err != nil {
		return err
	}
	if !review.MatchesExpected {
		return errors.New("source stack revision conflict; manual review required")
	}
	return nil
}
