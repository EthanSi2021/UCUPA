package admission

import (
	"context"
	"time"

	"ucupa/shared/values"
)

// One cancellation source belongs to the existing bounded session entry, not
// another registry. It allocates no timer or goroutine when a session is idle.
type sessionLifetime struct {
	context context.Context
	cancel  context.CancelFunc
}

func newSessionLifetime() *sessionLifetime {
	ctx, cancel := context.WithCancel(context.Background())
	return &sessionLifetime{context: ctx, cancel: cancel}
}

func (l *sessionLifetime) stop() {
	if l != nil {
		l.cancel()
	}
}

// EntryContext bounds a long-running operation by its original session. It
// holds no admission lock while calling an owner or waiting on a peer.
func (s *Service) EntryContext(ctx context.Context, entry EntrySession) (context.Context, context.CancelFunc, error) {
	if ctx == nil || entry.entry.lifetime == nil || s.CheckEntrySession(entry) != nil {
		return nil, nil, values.PublicDenied
	}
	bounded, cancel := context.WithDeadline(ctx, entry.Expires())
	lifetime := entry.entry.lifetime.context
	stop := context.AfterFunc(lifetime, cancel)
	deadline, _ := bounded.Deadline()
	out := sessionContext{Context: bounded, lifetime: lifetime, expires: deadline, cancel: cancel}
	return out, func() { stop(); cancel() }, nil
}

type sessionContext struct {
	context.Context
	lifetime context.Context
	expires  time.Time
	cancel   context.CancelFunc
}

// Cancellation notification is asynchronous. Final owner checks must still
// see ReleaseSession synchronously, even before the notification callback runs.
// Reading this context never takes admission's mutex under an owner lock.
func (ctx sessionContext) Err() error {
	if err := ctx.Context.Err(); err != nil {
		return err
	}
	if ctx.lifetime.Err() != nil || !time.Now().Before(ctx.expires) {
		// Close Done before returning a non-nil Err, preserving the context
		// contract even if the asynchronous notification has not run yet.
		ctx.cancel()
		return ctx.Context.Err()
	}
	return nil
}
