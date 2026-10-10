package main

import (
	"testing"

	"ucupa/shared/values"
)

func TestRealmProfilesAreExplicitAndNeverFallback(t *testing.T) {
	local := config{RealmStore: "realm.db", FacetStore: "facet.db", RealmNextHop: "local:realm"}
	network := config{RealmProfile: "network", FacetStore: "facet.db", RealmEndpoints: []realmEndpointConfig{{NextHop: "https://localhost:34567"}}}
	for _, c := range []config{local, network} {
		if err := c.checkRealmProfile(); err != nil {
			t.Fatalf("valid profile refused: %v", err)
		}
	}
	for _, mode := range []string{"network-empty", "network-local-store", "network-local-hop", "local-network-endpoint", "unknown", "unbounded"} {
		t.Run(mode, func(t *testing.T) {
			c := network
			switch mode {
			case "network-empty":
				c.RealmEndpoints = nil
			case "network-local-store":
				c.RealmStore = "realm.db"
			case "network-local-hop":
				c.RealmNextHop = values.NextHop("local:realm")
			case "local-network-endpoint":
				c = local
				c.RealmEndpoints = network.RealmEndpoints
			case "unknown":
				c.RealmProfile = "automatic"
			case "unbounded":
				c.RealmEndpoints = make([]realmEndpointConfig, 257)
			}
			if c.checkRealmProfile() == nil {
				t.Fatal("invalid/mixed profile was accepted")
			}
		})
	}
}
