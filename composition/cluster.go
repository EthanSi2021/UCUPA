// Package composition assembles the current owner modules without owning
// any of their domain facts.
package composition

import (
	"errors"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	core "ucupa/ucupa-core/instance"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	realminstance "ucupa/ucupa-realm/instance"
)

var ErrInvalidSeed = errors.New("invalid Composition seed")

// Seed describes one in-memory Cluster assembly around an explicitly provided
// Realm. Its caller retains Realm ownership and grants access after reading the
// newly created Core identity; Composition does not create or rewrite grants.
type Seed struct {
	ID                values.ClusterID
	Realm             *realminstance.Realm
	RealmNextHop      values.NextHop
	FacetServiceActor values.PrincipalRef
	CoreServiceActor  values.PrincipalRef
	CoreNextHop       values.NextHop
	FacetNextHop      values.NextHop
	CoreConfig        corecontract.Config
	FacetConfig       facet.Config
}

// Cluster is a shallow assembly root. The referenced modules retain exclusive
// authority over their own facts.
type Cluster struct {
	id               values.ClusterID
	policy           *realminstance.Realm
	realmConnections *LocalRealmConnections
	core             *core.Core
	facets           *facet.Instance
	facetConnections *LocalFacetConnections
}

func NewCluster(seed Seed) (*Cluster, error) {
	if seed.ID == "" || seed.Realm == nil || seed.Realm.ID() == "" || !seed.RealmNextHop.IsLocal() || !seed.FacetServiceActor.Valid() || !seed.CoreServiceActor.Valid() || !seed.CoreNextHop.IsLocal() || !seed.FacetNextHop.IsLocal() {
		return nil, ErrInvalidSeed
	}

	policy := seed.Realm
	realmConnections := &LocalRealmConnections{}
	if err := realmConnections.BindEndpoint(seed.RealmNextHop, policy); err != nil {
		return nil, err
	}
	coreInitial := corecontract.RealmInitialization{Realm: policy.ID(), NextHop: seed.RealmNextHop}
	spatial, err := core.New(realmConnections, coreInitial, seed.CoreConfig)
	if err != nil {
		return nil, err
	}
	coreConnections := &LocalFacetCoreConnections{}
	if err = coreConnections.BindEndpoint(seed.CoreNextHop, LocalFacetCoreEndpoint{Read: spatial.Facets(), Control: spatial.Facets(), Access: spatial.Access()}); err != nil {
		return nil, err
	}
	caller, err := realmConnections.TrustLocalCaller(seed.RealmNextHop, policy.ID(), seed.FacetServiceActor)
	if err != nil {
		_ = spatial.Close()
		return nil, err
	}
	source, err := coreConnections.ForFacetCaller(caller)
	if err != nil {
		return nil, err
	}
	facetInitial := facetcontract.RealmInitialization{Realm: policy.ID(), NextHop: seed.RealmNextHop}
	facets, err := facet.New(realmConnections, source, facetInitial, seed.FacetConfig)
	if err != nil {
		return nil, err
	}
	facetConnections := &LocalFacetConnections{}
	if err = facetConnections.BindEndpoint(seed.FacetNextHop, facets); err != nil {
		return nil, err
	}
	acceptance, err := facetConnections.ForCore(seed.CoreServiceActor)
	if err != nil {
		return nil, err
	}
	if err = spatial.Facets().SetFacetAcceptanceSource(acceptance); err != nil {
		return nil, err
	}
	coreCaller, err := realmConnections.TrustLocalCaller(seed.RealmNextHop, policy.ID(), seed.CoreServiceActor)
	if err != nil {
		return nil, err
	}
	disposition, err := facetConnections.ForCoreCaller(coreCaller)
	if err != nil {
		return nil, err
	}
	if err = spatial.Facets().SetFacetDispositionSource(disposition); err != nil {
		return nil, err
	}
	return &Cluster{id: seed.ID, policy: policy, realmConnections: realmConnections, core: spatial, facets: facets, facetConnections: facetConnections}, nil
}

func (cluster *Cluster) ID() values.ClusterID {
	if cluster == nil {
		return ""
	}
	return cluster.id
}

// RealmID identifies the independently owned policy namespace wired into this
// Cluster. Composition does not expose Realm itself or interpret its policy.
func (cluster *Cluster) RealmID() values.RealmID {
	if cluster == nil || cluster.policy == nil {
		return ""
	}
	return cluster.policy.ID()
}

func (cluster *Cluster) Core() *core.Core {
	if cluster == nil {
		return nil
	}
	return cluster.core
}

func (cluster *Cluster) Facets() *facet.Instance {
	if cluster == nil {
		return nil
	}
	return cluster.facets
}

// FacetConnections exposes only rebuildable local address bindings.
func (c *Cluster) FacetConnections() *LocalFacetConnections { return c.facetConnections }

// RealmConnections exposes only the rebuildable local Realm address binding
// used by this Cluster's owners. It is not a Realm directory and grants no
// authorization facts.
func (cluster *Cluster) RealmConnections() *LocalRealmConnections {
	if cluster == nil {
		return nil
	}
	return cluster.realmConnections
}
