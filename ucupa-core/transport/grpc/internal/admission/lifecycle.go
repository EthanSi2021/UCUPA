package admission

import (
	"errors"
	"net"
	"time"

	"ucupa/internal/grpcbound"
)

// BindListener shares the admission lock with Stop, retaining the original
// single transition between first Serve, admitted RPCs and shutdown.
func (s *Service) BindListener(l net.Listener, maximum int, lifetime time.Duration) (*grpcbound.Listener, error) {
	if l == nil {
		return nil, errors.New("listener required")
	}
	s.mu.Lock()
	if s.stopped || s.listener != nil {
		s.mu.Unlock()
		_ = l.Close()
		return nil, errors.New("service already serving or stopped")
	}
	limited := grpcbound.NewListener(l, maximum, lifetime)
	s.listener = limited
	s.mu.Unlock()
	return limited, nil
}

// BeginStop closes admission before the host waits for RPCs and closes the
// network. As before, pending reservations settle in their invocation defers;
// approval preparation observes stopped while holding this same mutex.
func (s *Service) BeginStop() *grpcbound.Listener {
	s.mu.Lock()
	s.stopped = true
	for _, entry := range s.sessions {
		entry.lifetime.stop()
	}
	clear(s.sessions)
	clear(s.grants)
	clear(s.challenges)
	clear(s.transitionApprovals)
	listener := s.listener
	s.mu.Unlock()
	return listener
}

func (s *Service) Wait() { s.running.Wait() }
