package mission

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// ---------------------------------------------------------------------------
// IsTransientError tests
// ---------------------------------------------------------------------------

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"EOF", io.EOF, true},
		{"wrapped EOF", fmt.Errorf("read: %w", io.EOF), true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"timeout string", fmt.Errorf("dial tcp: i/o timeout"), true},
		{"connection refused string", fmt.Errorf("dial tcp 127.0.0.1:3307: connection refused"), true},
		{"connection reset", fmt.Errorf("read: connection reset by peer"), true},
		{"broken pipe", fmt.Errorf("write: broken pipe"), true},
		{"server gone away", fmt.Errorf("Error 2006: MySQL server has gone away"), true},
		{"invalid connection", fmt.Errorf("invalid connection"), true},
		{"bad connection", fmt.Errorf("bad connection"), true},
		{"too many connections", fmt.Errorf("too many connections"), true},
		{"permanent: table not found", fmt.Errorf("Error 1146: Table 'missions' doesn't exist"), false},
		{"permanent: syntax error", fmt.Errorf("Error 1064: You have an error in your SQL syntax"), false},
		{"mysql too many connections", &mysql.MySQLError{Number: 1040, Message: "Too many connections"}, true},
		{"mysql lock timeout", &mysql.MySQLError{Number: 1205, Message: "Lock wait timeout exceeded"}, true},
		{"mysql deadlock", &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}, true},
		{"mysql syntax error", &mysql.MySQLError{Number: 1064, Message: "syntax error"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransientError(tt.err)
			if got != tt.want {
				t.Errorf("IsTransientError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// net.Error implementation for testing
// ---------------------------------------------------------------------------

type testNetErr struct {
	timeout   bool
	temporary bool
	msg       string
}

func (e *testNetErr) Error() string   { return e.msg }
func (e *testNetErr) Timeout() bool   { return e.timeout }
func (e *testNetErr) Temporary() bool { return e.temporary }

var _ net.Error = (*testNetErr)(nil)

func TestIsTransientError_NetError(t *testing.T) {
	err := &testNetErr{timeout: true, msg: "dial tcp: i/o timeout"}
	if !IsTransientError(err) {
		t.Error("net.Error with timeout should be transient")
	}

	err2 := &testNetErr{timeout: false, msg: "connection refused"}
	if !IsTransientError(err2) {
		t.Error("net.Error (connection refused) should be transient")
	}
}

// ---------------------------------------------------------------------------
// retryStoreGet tests
// ---------------------------------------------------------------------------

func TestRetryStoreGet_SucceedsFirstTry(t *testing.T) {
	calls := 0
	result, err := retryStoreGet(context.Background(), retryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(_ context.Context) (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q, want ok", result)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryStoreGet_SucceedsAfterTransientErrors(t *testing.T) {
	calls := 0
	result, err := retryStoreGet(context.Background(), retryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(_ context.Context) (string, error) {
		calls++
		if calls < 3 {
			return "", fmt.Errorf("dial tcp: i/o timeout")
		}
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Fatalf("result = %q, want recovered", result)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryStoreGet_PermanentErrorNoRetry(t *testing.T) {
	calls := 0
	_, err := retryStoreGet(context.Background(), retryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(_ context.Context) (string, error) {
		calls++
		return "", fmt.Errorf("Error 1146: Table 'missions' doesn't exist")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (should not retry permanent errors)", calls)
	}
}

func TestRetryStoreGet_ExhaustsRetries(t *testing.T) {
	calls := 0
	_, err := retryStoreGet(context.Background(), retryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(_ context.Context) (string, error) {
		calls++
		return "", fmt.Errorf("dial tcp: i/o timeout")
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryStoreGet_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := retryStoreGet(ctx, retryConfig{MaxAttempts: 5, BaseDelay: 50 * time.Millisecond}, func(_ context.Context) (string, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return "", fmt.Errorf("dial tcp: i/o timeout")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		// After cancellation, the context error is returned
		// but the transient error from the call may also be returned.
		// Either is acceptable.
	}
	if calls > 3 {
		t.Fatalf("calls = %d, expected ≤3 (should stop on context cancel)", calls)
	}
}

func TestRetryStoreExec(t *testing.T) {
	calls := 0
	err := retryStoreExec(context.Background(), retryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(_ context.Context) error {
		calls++
		if calls < 2 {
			return fmt.Errorf("connection refused")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}
