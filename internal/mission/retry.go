package mission

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// IsTransientError returns true if the error is a transient Dolt/network
// error that is likely to resolve on retry (timeout, connection refused,
// EOF, etc.). Permanent errors (bad SQL, missing table) return false.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation is not retryable.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Network-level errors (timeout, connection refused, reset).
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Connection closed / broken pipe.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// MySQL driver errors — check error codes.
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1040: // Too many connections
			return true
		case 1205: // Lock wait timeout
			return true
		case 1213: // Deadlock
			return true
		case 2002, 2003, 2006, 2013: // Connection errors
			return true
		}
		return false // Other MySQL errors are permanent (bad query, missing table, etc.).
	}

	// String-based fallback for wrapped errors.
	msg := strings.ToLower(err.Error())
	for _, pattern := range []string{
		"i/o timeout",
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"too many connections",
		"server has gone away",
		"invalid connection",
		"bad connection",
	} {
		if strings.Contains(msg, pattern) {
			return true
		}
	}

	return false
}

// retryConfig controls retry behavior for store operations.
type retryConfig struct {
	MaxAttempts int           // Total attempts (1 = no retry). Default: 3.
	BaseDelay   time.Duration // Initial delay before first retry. Default: 100ms.
	MaxDelay    time.Duration // Cap on exponential backoff. Default: 2s.
}

func (c *retryConfig) applyDefaults() {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 3
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = 100 * time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 2 * time.Second
	}
}

// retryStoreGet retries a store read operation with exponential backoff,
// only retrying on transient errors. Returns the last error on exhaustion.
func retryStoreGet[T any](ctx context.Context, cfg retryConfig, op func(context.Context) (T, error)) (T, error) {
	cfg.applyDefaults()

	var lastErr error
	var zero T
	delay := cfg.BaseDelay

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		result, err := op(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// Don't retry permanent errors or context cancellation.
		if !IsTransientError(err) || ctx.Err() != nil {
			return zero, err
		}

		// Last attempt — don't sleep.
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}

		// Exponential backoff with cap.
		delay *= 2
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}
	return zero, lastErr
}

// retryStoreExec retries a store write/void operation with exponential backoff.
func retryStoreExec(ctx context.Context, cfg retryConfig, op func(context.Context) error) error {
	_, err := retryStoreGet(ctx, cfg, func(c context.Context) (struct{}, error) {
		return struct{}{}, op(c)
	})
	return err
}
