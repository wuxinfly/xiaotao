// Package conformance checks a built plugin executable against the protocol the
// host expects. It spawns the plugin the same way the host does and only uses
// documented messages, so third-party authors can run it before shipping.
package conformance

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"xiaotao/xt-client/sdk/plugin"
)

// Check is one protocol expectation.
type Check struct {
	Name string
	Err  error
}

// Result is the full run, in order.
type Result struct {
	Checks []Check
}

// OK reports whether every check passed.
func (r Result) OK() bool {
	for _, check := range r.Checks {
		if check.Err != nil {
			return false
		}
	}
	return true
}

// Run spawns exe and verifies handshake, unknown-method handling, stdout purity
// and clean shutdown. It never sends credentials or chat bodies.
func Run(ctx context.Context, exe, wantID string) Result {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	result := Result{}
	cmd := exec.CommandContext(ctx, exe)
	cmd.Dir = "."
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{Checks: []Check{{Name: "start", Err: err}}}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{Checks: []Check{{Name: "start", Err: err}}}
	}
	if err := cmd.Start(); err != nil {
		return Result{Checks: []Check{{Name: "start", Err: err}}}
	}
	defer func() { _ = stdin.Close() }()
	reader := bufio.NewScanner(stdout)
	reader.Buffer(make([]byte, 4096), plugin.MaxMessage)

	// Handshake.
	if err := write(stdin, plugin.Message{JSONRPC: "2.0", ID: 1, Method: "initialize", Params: json.RawMessage(fmt.Sprintf(`{"contractVersion":%d,"minContractVersion":1}`, plugin.ContractVersion))}); err != nil {
		return Result{Checks: []Check{{Name: "initialize write", Err: err}}}
	}
	reply, err := read(reader)
	result.Checks = append(result.Checks, Check{Name: "initialize replies", Err: err})
	if err == nil {
		var init struct {
			ID              string `json:"id"`
			ContractVersion int    `json:"contractVersion"`
		}
		decodeErr := json.Unmarshal(reply.Result, &init)
		if decodeErr == nil && wantID != "" && init.ID != wantID {
			decodeErr = fmt.Errorf("id %q does not match manifest %q", init.ID, wantID)
		}
		if decodeErr == nil && (init.ContractVersion < 1 || init.ContractVersion > plugin.ContractVersion) {
			decodeErr = fmt.Errorf("contractVersion %d outside 1..%d", init.ContractVersion, plugin.ContractVersion)
		}
		result.Checks = append(result.Checks, Check{Name: "initialize identity and version", Err: decodeErr})
	}

	// Unknown methods must be refused, not silently accepted.
	if err := write(stdin, plugin.Message{JSONRPC: "2.0", ID: 2, Method: "conformance/unknown", Params: json.RawMessage(`{}`)}); err != nil {
		return Result{Checks: []Check{{Name: "unknown method write", Err: err}}}
	}
	reply, err = read(reader)
	result.Checks = append(result.Checks, Check{Name: "unknown method replies", Err: err})
	if err == nil {
		var unsupported error
		if reply.Error == nil {
			unsupported = errors.New("unknown method returned a result")
		} else if reply.Error.Code != plugin.CodeUnsupported {
			unsupported = fmt.Errorf("unknown method code %d, want %d", reply.Error.Code, plugin.CodeUnsupported)
		}
		result.Checks = append(result.Checks, Check{Name: "unknown method uses unsupported code", Err: unsupported})
	}

	// Malformed input must end the process instead of being ignored.
	_, _ = io.WriteString(stdin, "not json\n")
	_ = stdin.Close()
	exited := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, stdout)
		_ = cmd.Wait()
		close(exited)
	}()
	select {
	case <-exited:
		result.Checks = append(result.Checks, Check{Name: "malformed input stops the process"})
	case <-time.After(5 * time.Second):
		result.Checks = append(result.Checks, Check{Name: "malformed input stops the process", Err: errors.New("process kept running")})
	}
	return result
}

func write(w io.Writer, message plugin.Message) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

func read(scanner *bufio.Scanner) (plugin.Message, error) {
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return plugin.Message{}, err
		}
		return plugin.Message{}, errors.New("plugin closed stdout")
	}
	var message plugin.Message
	if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
		return plugin.Message{}, fmt.Errorf("stdout is not pure JSON-RPC: %w", err)
	}
	if message.JSONRPC != "2.0" {
		return plugin.Message{}, errors.New("missing jsonrpc version")
	}
	return message, nil
}
