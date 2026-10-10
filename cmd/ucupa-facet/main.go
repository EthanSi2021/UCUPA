// Command ucupa-facet serves one existing durable Facet and independently
// contacts configured Core services. It never initializes owner facts or grants.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"ucupa/composition"
	facetcontract "ucupa/ucupa-facet/contract"
	facet "ucupa/ucupa-facet/instance"
	facetgrpc "ucupa/ucupa-facet/transport/grpc"
	realminstance "ucupa/ucupa-realm/instance"
)

func main() {
	path := flag.String("config", "", "required service configuration file")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: ucupa-facet -config <file>")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *path); err != nil {
		fmt.Fprintln(os.Stderr, "ucupa-facet:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, path string) error {
	c, err := readConfig(path)
	if err != nil {
		return err
	}
	var realmConnections interface {
		facetcontract.RealmConnections
		composition.RealmCallerSource
	}
	if c.RealmProfile == "network" {
		connections := &composition.NetworkRealmConnections{}
		for _, binding := range c.realmBindings {
			if err := connections.BindNetworkEndpoint(binding.address, c.RealmID, binding.material); err != nil {
				return errors.New("cannot bind pinned Realm endpoint")
			}
			defer connections.UnbindNetworkEndpoint(binding.address)
		}
		realmConnections = connections
	} else {
		policy, err := realminstance.OpenDurable(c.RealmStore)
		if err != nil {
			return errors.New("cannot open Realm Store")
		}
		defer policy.Close()
		if policy.ID() != c.RealmID {
			return errors.New("Realm identity mismatch")
		}
		connections := &composition.LocalRealmConnections{}
		if err := connections.BindEndpoint(c.RealmNextHop, policy); err != nil {
			return errors.New("cannot bind local Realm address")
		}
		realmConnections = connections
	}
	source, closeClients, err := c.openCoreSource()
	if err != nil {
		return err
	}
	defer closeClients()
	owner, err := facet.OpenDurable(realmConnections, source, c.FacetStore, c.ownerConfig)
	if err != nil {
		return errors.New("cannot open Facet Store")
	}
	defer owner.Close()
	server, err := facetgrpc.New(owner.Namespace, composition.RealmNetworkAuthentication{Connections: realmConnections, Owner: owner.Namespace}, c.networkConfig)
	if err != nil {
		return err
	}
	defer server.Stop()
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return errors.New("cannot listen on configured address")
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Println("ucupa-facet: ready", c.NextHop)
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		if errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return err
	}
}
