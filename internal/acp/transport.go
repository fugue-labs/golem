package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Transport handles JSON-RPC 2.0 message framing over stdio.
// Messages are newline-delimited JSON (one JSON object per line).
type Transport struct {
	in  *bufio.Scanner
	out io.Writer
	mu  sync.Mutex // serializes writes
}

// NewTransport creates a transport reading from r and writing to w.
func NewTransport(r io.Reader, w io.Writer) *Transport {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10 MB max line
	return &Transport{in: scanner, out: w}
}

// ReadRequest reads the next JSON-RPC request from stdin.
// Returns io.EOF when the input stream closes.
func (t *Transport) ReadRequest() (*Request, error) {
	for t.in.Scan() {
		line := t.in.Bytes()
		if len(line) == 0 {
			continue // skip blank lines
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return &req, nil
	}
	if err := t.in.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// SendResponse writes a JSON-RPC response.
func (t *Transport) SendResponse(resp Response) error {
	resp.JSONRPC = "2.0"
	return t.write(resp)
}

// SendNotification writes a JSON-RPC notification.
func (t *Transport) SendNotification(notif Notification) error {
	notif.JSONRPC = "2.0"
	return t.write(notif)
}

// SendRequest writes a JSON-RPC request (agent → client).
func (t *Transport) SendRequest(req Request) error {
	req.JSONRPC = "2.0"
	return t.write(req)
}

func (t *Transport) write(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	_, err = t.out.Write(data)
	return err
}
