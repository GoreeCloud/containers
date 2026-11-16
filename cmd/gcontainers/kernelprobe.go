package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/kernelprobe"
	"io"
	"os"
	"runtime"
	"time"
)

func runKernelProbe(out io.Writer) error {
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		return errors.New("rootless Linux caller required")
	}
	// proc/self/exe pins the already-running program inode, rather than
	// resolving a potentially replaced user-writable executable pathname.
	executable := "/proc/self/exe"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report, err := kernelprobe.Probe(ctx, executable)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
func runKernelProbeChild(out io.Writer) error {
	if runtime.GOOS != "linux" {
		return errors.New("Linux namespace probe only")
	}
	b, err := kernelprobe.WriteChildComparison(os.Stdin)
	if err != nil {
		return err
	}
	_, err = out.Write(b)
	return err
}
