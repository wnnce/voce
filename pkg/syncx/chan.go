package syncx

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSendBlocked = errors.New("send operation blocked")
)

func SendWithContext[T any](ctx context.Context, ch chan<- T, value T) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- value:
		return nil
	}
}

func SendWithTimeout[T any](ctx context.Context, ch chan<- T, value T, duration time.Duration) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	return SendWithContext(timeoutCtx, ch, value)
}

func SendWithNonBlocking[T any](ctx context.Context, ch chan<- T, value T) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ch <- value:
		return nil
	default:
		return ErrSendBlocked
	}
}
