// Package instance constructs and opens one Realm owner. Business methods
// operate on the actual policy handle, not on a parallel forwarding facade.
package instance

import (
	"ucupa/ucupa-realm/contract"
	"ucupa/ucupa-realm/internal/policy"
)

type Realm = policy.Realm

func New(seed contract.Seed) (*Realm, error) { return policy.New(seed) }
func CreateDurable(seed contract.Seed, path string) (*Realm, error) {
	return policy.CreateDurable(seed, path)
}
func OpenDurable(path string) (*Realm, error) { return policy.OpenDurable(path) }
