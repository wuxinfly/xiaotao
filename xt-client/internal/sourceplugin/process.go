package sourceplugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// Message uses newline-delimited JSON-RPC 2.0. Stdout is protocol only.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Process owns its reader and waiter; Close reaps the direct child. Notifications
// are consumed synchronously and must not call Call on the same reader goroutine.
type Process struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	write   sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan Message
	done    chan struct{}
	exited  chan struct{}
	stop    sync.Once
	seq     atomic.Uint64
	notify  func(Message)
}

func Start(ctx context.Context, dir string, notify func(Message)) (*Process, error) {
	m, entry, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, entry)
	cmd.Dir = dir
	// Do not forward stderr to application logs: third-party output may contain secrets.
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		return nil, err
	}
	p := &Process{cmd: cmd, in: in, pending: map[uint64]chan Message{}, done: make(chan struct{}), exited: make(chan struct{}), notify: notify}
	readDone := make(chan struct{})
	go func() { p.read(out); close(readDone) }()
	go func() { <-readDone; _ = cmd.Wait(); p.fail(); close(p.exited) }()
	var initialized struct {
		ContractVersion int    `json:"contractVersion"`
		ID              string `json:"id"`
	}
	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	err = p.Call(initCtx, "initialize", map[string]any{"contractVersion": ContractVersion}, &initialized)
	if err != nil || initialized.ContractVersion != ContractVersion || initialized.ID != m.ID {
		p.Close()
		return nil, errors.New("plugin initialization failed")
	}
	return p, nil
}
func (p *Process) fail() {
	p.stop.Do(func() { close(p.done); _ = p.in.Close(); _ = p.cmd.Process.Kill() })
}
func (p *Process) Close() error { p.fail(); <-p.exited; return nil }
func (p *Process) send(m Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(raw) > MaxMessage {
		return errors.New("plugin message too large")
	}
	p.write.Lock()
	defer p.write.Unlock()
	_, err = p.in.Write(append(raw, '\n'))
	return err
}
func (p *Process) Call(ctx context.Context, method string, params any, result any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	id := p.seq.Add(1)
	reply := make(chan Message, 1)
	p.mu.Lock()
	p.pending[id] = reply
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	if err = p.send(Message{JSONRPC: "2.0", ID: id, Method: method, Params: raw}); err != nil {
		return errors.New("plugin write failed")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return errors.New("plugin process terminated")
	case m := <-reply:
		if m.Error != nil {
			return errors.New("plugin operation failed")
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(m.Result, result)
	}
}
func (p *Process) Notify(method string, params any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return p.send(Message{JSONRPC: "2.0", Method: method, Params: raw})
}
func (p *Process) read(out io.Reader) {
	defer p.fail()
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), MaxMessage)
	for scanner.Scan() {
		var m Message
		if json.Unmarshal(scanner.Bytes(), &m) != nil || m.JSONRPC != "2.0" {
			return
		}
		if m.Method != "" {
			if m.ID != 0 {
				return
			}
			if p.notify != nil {
				p.notify(m)
			}
			continue
		}
		if m.ID == 0 || (m.Error == nil && m.Result == nil) || (m.Error != nil && m.Result != nil) {
			return
		}
		p.mu.Lock()
		ch := p.pending[m.ID]
		if ch != nil {
			delete(p.pending, m.ID)
		}
		p.mu.Unlock()
		if ch != nil {
			ch <- m
		}
	}
}
