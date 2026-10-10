package admission

import (
	"crypto/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ucupa/shared/values"
)

type Config struct {
	Expected                             values.RealmID
	AdmittedServices                     map[[32]byte]values.FactAuthorityRef
	MaxConcurrentRPCs                    int
	ChallengeTTL                         time.Duration
	MaxChallenges, MaxChallengesPerOwner int
}

type challengeEntry struct {
	owner   values.FactAuthorityRef
	expires time.Time
}

func (r *Registry) NewChallenge(owner values.FactAuthorityRef) ([32]byte, error) {
	var challenge [32]byte
	if _, err := rand.Read(challenge[:]); err != nil {
		return challenge, status.Error(codes.Unavailable, "entropy unavailable")
	}
	now := r.now()
	r.gateMu.Lock()
	defer r.gateMu.Unlock()
	for key, entry := range r.challenges {
		if !now.Before(entry.expires) {
			delete(r.challenges, key)
		}
	}
	if len(r.challenges) >= r.config.MaxChallenges {
		return challenge, status.Error(codes.ResourceExhausted, "challenge capacity")
	}
	pending := 0
	for _, entry := range r.challenges {
		if entry.owner == owner {
			pending++
		}
	}
	if pending >= r.config.MaxChallengesPerOwner {
		return challenge, status.Error(codes.ResourceExhausted, "challenge capacity")
	}
	r.challenges[challenge] = challengeEntry{owner: owner, expires: now.Add(r.config.ChallengeTTL)}
	return challenge, nil
}

func (r *Registry) HasChallenge(challenge [32]byte, owner values.FactAuthorityRef, now time.Time) bool {
	r.gateMu.Lock()
	defer r.gateMu.Unlock()
	entry, pending := r.challenges[challenge]
	if pending && (entry.owner != owner || !now.Before(entry.expires)) {
		delete(r.challenges, challenge)
		pending = false
	}
	return pending
}

// ConsumeChallenge rechecks and removes under one lock after the liveness wait.
func (r *Registry) ConsumeChallenge(challenge [32]byte, owner values.FactAuthorityRef) error {
	now := r.now()
	r.gateMu.Lock()
	defer r.gateMu.Unlock()
	entry, ok := r.challenges[challenge]
	if !ok || entry.owner != owner || !now.Before(entry.expires) {
		return status.Error(codes.Unauthenticated, "challenge rejected")
	}
	delete(r.challenges, challenge)
	return nil
}

func (r *Registry) Stopped() bool {
	r.gateMu.Lock()
	defer r.gateMu.Unlock()
	return r.stopped
}

func (r *Registry) Stop() {
	r.gateMu.Lock()
	r.stopped = true
	clear(r.challenges)
	r.gateMu.Unlock()
	r.Reset()
}
