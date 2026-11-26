package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/localapi"
	"io"
	"time"
)

func localQuery(args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: local-query ABS_SOCKET health|journal|runtime-summary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	data, status, err := localapi.Query(ctx, args[0], args[1])
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintln(out, string(data)); err != nil {
		return err
	}
	if status == 409 {
		return errors.New("recovery review required")
	}
	return nil
}
