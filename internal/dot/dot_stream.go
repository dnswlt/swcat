package dot

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

// streamingRunner is an implementation of Runner that keeps a single dot process
// running in the background and feeds it requests via stdin/stdout.
// This is significantly faster on Windows where process creation overhead is high.
type streamingRunner struct {
	dotPath string
	mu      sync.Mutex // protects access to the running process
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *bytes.Buffer // capture stderr for debugging
}

// NewStreamingRunner creates a new Runner that uses a persistent dot process.
func NewStreamingRunner(dotPath string) Runner {
	return &streamingRunner{
		dotPath: dotPath,
	}
}

// ensureProcessStarted makes sure the dot process is running.
// Caller must hold the mutex.
func (r *streamingRunner) ensureProcessStarted() error {
	if r.cmd != nil {
		if r.cmd.ProcessState == nil && r.cmd.Process != nil {
			// Check if process is still alive. This is a bit rough, but
			// if Write fails later we will restart anyway.
			return nil
		}
		// Process is dead or finished, restart
		r.stopProcess()
	}

	// Start dot process.
	// We rely on the empirical observation that `dot` does support multiple graphs in one input stream.
	// If you pipe:
	// digraph G1 { ... }
	// digraph G2 { ... }
	//
	// to `dot -Tsvg`, it will output two SVG documents concatenated.
	// The problem is knowing when one document ends.
	// SVG output is XML, so it ends with </svg>. We can scan for that.

	r.cmd = exec.Command(r.dotPath, "-Tsvg")
	r.stderr = &bytes.Buffer{}
	r.cmd.Stderr = r.stderr

	var err error
	r.stdin, err = r.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("getting stdin pipe: %w", err)
	}

	stdoutPipe, err := r.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("getting stdout pipe: %w", err)
	}
	r.stdout = bufio.NewReader(stdoutPipe)

	log.Printf("Starting background dot process (%s)", r.dotPath)
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("starting dot process: %w", err)
	}

	return nil
}

func (r *streamingRunner) stopProcess() {
	if r.stdin != nil {
		r.stdin.Close()
	}
	if r.cmd != nil && r.cmd.Process != nil {
		r.cmd.Process.Kill() // Force kill if needed
		r.cmd.Wait()
	}
	r.cmd = nil
	r.stdin = nil
	r.stdout = nil
	r.stderr = nil
}

// Run executes the dot command for the given source using the streaming process.
func (r *streamingRunner) Run(ctx context.Context, dotSource string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	started := time.Now()

	// 1. Ensure process is running
	if err := r.ensureProcessStarted(); err != nil {
		return nil, err
	}

	// 2. Send input
	_, err := io.WriteString(r.stdin, dotSource+"\n")
	if err != nil {
		log.Printf("Writing to dot process failed (%v), restarting...", err)
		r.stopProcess()
		if err := r.ensureProcessStarted(); err != nil {
			return nil, fmt.Errorf("failed to restart dot process: %w", err)
		}
		if _, err := io.WriteString(r.stdin, dotSource+"\n"); err != nil {
			return nil, fmt.Errorf("writing to dot process failed after restart: %w", err)
		}
	}

	// 3. Read output with cancellation/timeout support
	type readResult struct {
		data []byte
		err  error
	}
	done := make(chan readResult, 1)

	go func() {
		var output bytes.Buffer
		for {
			line, err := r.stdout.ReadBytes('\n')
			if err != nil {
				// Process died or pipe closed
				done <- readResult{nil, fmt.Errorf("dot process exited unexpectedly: %w", err)}
				return
			}
			output.Write(line)
			if bytes.Contains(line, []byte("</svg>")) {
				done <- readResult{output.Bytes(), nil}
				return
			}
		}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			// If reading failed, the process is likely dead or in bad state.
			r.stopProcess()
			return nil, res.err
		}

		outBytes := res.data

		elapsed := time.Since(started).Milliseconds()
		log.Printf("Generated SVG (%d bytes) using streaming dot in %d ms", len(outBytes), elapsed)

		if idx := bytes.Index(outBytes, []byte("<svg")); idx > 0 {
			outBytes = outBytes[idx:]
		}

		return outBytes, nil

	case <-ctx.Done():
		// Context timeout or cancellation.
		// We MUST kill the process because it might be stuck reading (if we didn't send enough data)
		// or writing (if we are not reading fast enough, though we are in a goroutine).
		// More likely: it's stuck because input was invalid (syntax error) and it's waiting for more input
		// or it printed to stderr and is waiting for next graph command while we wait for stdout.
		// In any case, state is desynchronized.
		r.stopProcess()
		return nil, ctx.Err()
	}
}

// Close cleans up the running process.
func (r *streamingRunner) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopProcess()
	return nil
}
