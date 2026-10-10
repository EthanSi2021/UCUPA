// Command ucupa-core serves one already initialized durable Core over gRPC.
// Initialization, credentials, and permission provisioning are separate owner operations.
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
	"ucupa/shared/values"
	core "ucupa/ucupa-core/instance"
	coregrpc "ucupa/ucupa-core/transport/grpc"
	realm "ucupa/ucupa-realm/contract"
	realminstance "ucupa/ucupa-realm/instance"
)

func main() {
	config := flag.String("config", "", "required service configuration file")
	flag.Parse()
	if *config == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: ucupa-core -config <file>")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *config); err != nil {
		fmt.Fprintln(os.Stderr, "ucupa-core:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, path string) error {
	c, err := readConfig(path)
	if err != nil {
		return err
	}
	// One explicitly configured Realm profile. The local profile binds the
	// in-process kernel; the network profile pins the serving Realm's TLS
	// materials per address. Neither profile creates, replaces or supplies the
	// persisted contact — open recovers it from the original Store and
	// verifies it online before serving.
	var policy *realminstance.Realm
	var localConnections *composition.LocalRealmConnections
	var networkConnections *composition.NetworkRealmConnections
	if c.realmProfileNetwork() {
		networkConnections = &composition.NetworkRealmConnections{}
		for _, binding := range c.realmBindings {
			if err := networkConnections.BindNetworkEndpoint(binding.address, values.RealmID(c.RealmID), binding.material); err != nil {
				return errors.New("cannot bind pinned Realm endpoint")
			}
		}
	} else {
		policy, err = realminstance.OpenDurable(c.RealmStore)
		if err != nil {
			return errors.New("cannot open Realm Store")
		}
		defer policy.Close()
		if string(policy.ID()) != c.RealmID {
			return errors.New("Realm identity mismatch")
		}
		localConnections = &composition.LocalRealmConnections{}
		if err := localConnections.BindEndpoint(c.RealmNextHop, policy); err != nil {
			return errors.New("cannot bind local Realm address")
		}
	}
	var owner *core.Core
	if networkConnections != nil {
		owner, err = core.OpenDurable(networkConnections, c.CoreStore, c.coreConfig)
	} else {
		owner, err = core.OpenDurable(localConnections, c.CoreStore, c.coreConfig)
	}
	if err != nil {
		return errors.New("cannot open Core Store")
	}
	defer owner.Close()
	auth := composition.RealmNetworkAuthentication{Owner: owner.Access()}
	if networkConnections != nil {
		auth.Connections = networkConnections
	} else {
		auth.Connections = localConnections
	}
	c.networkConfig.MountControl = owner.Mounts()
	if c.FacetConnections != nil {
		source, closeClients, err := c.openFacetSource()
		if err != nil {
			return err
		}
		defer closeClients()
		if err = owner.Facets().SetFacetAcceptanceSource(source); err != nil {
			return err
		}
		if err = owner.Facets().SetFacetDispositionSource(source); err != nil {
			return err
		}
		// The fixed Core service identity of the replacement contacts comes
		// from the same trusted Facet endpoints; recovery never adopts
		// another principal or address.
		c.networkConfig.ReplacementPeers = source.ForReplacement()
	}
	if c.MountConnections != nil {
		connections, closeClients, err := openMountConnections(c.MountConnections, c.networkConfig)
		if err != nil {
			return err
		}
		defer closeClients()
		c.networkConfig.MountConnections = connections
		if err := owner.Mounts().ConfigureMountTransport(connections); err != nil {
			return err
		}
	}
	if c.IdentityIssuer != nil {
		var source realm.CallerSource
		if networkConnections != nil {
			// The network profile derives proof subjects through the owner's
			// current pinned contact; the local profile keeps the kernel.
			source = auth
		} else {
			source = policy
		}
		issuer, err := c.IdentityIssuer.open(source)
		if err != nil {
			return err
		}
		defer issuer.Close()
		c.networkConfig.IdentityProofs = composition.RealmIdentityProofs{Issuer: issuer}
	}
	if c.ExternalIdentity != nil {
		var external *composition.RealmExternalIdentity
		var closeClients func()
		if networkConnections != nil {
			external, closeClients, err = c.ExternalIdentity.open(networkConnections, owner.Access(), c.networkConfig)
		} else {
			external, closeClients, err = c.ExternalIdentity.open(localConnections, owner.Access(), c.networkConfig)
		}
		if err != nil {
			return err
		}
		defer closeClients()
		c.networkConfig.ExternalIdentity = external
	}
	if networkConnections != nil {
		// The network profile mints identity challenges at the serving Realm
		// so each proof's challenge is registered where it is verified; the
		// local profile keeps the in-process mint.
		c.networkConfig.ChallengeSource = composition.RealmChallengeSource{Connections: networkConnections, Owner: owner.Access()}
	}
	if c.Federation != nil {
		connections, err := c.Federation.open(c.networkConfig)
		if err != nil {
			return err
		}
		defer connections.Close()
		c.networkConfig.FederationConnections = connections
	}
	c.networkConfig.FacetControl = owner.Facets()
	c.networkConfig.WriterControl = owner.Writer()
	c.networkConfig.TransitionControl = owner.Transfers()
	c.networkConfig.TransitionSource = owner.Transfers()
	c.networkConfig.TransitionParent = owner.Transfers()
	c.networkConfig.TransitionImport = owner.Transfers()
	c.networkConfig.CarrierApproval = owner.Transfers()
	if c.TransitionConnections != nil {
		connections, closeClients, err := c.TransitionConnections.open(c.networkConfig)
		if err != nil {
			return err
		}
		defer closeClients()
		c.networkConfig.TransitionConnections = connections
		c.networkConfig.TransitionTarget = owner.Transfers()
		if c.networkConfig.CarrierApprovals != nil {
			c.networkConfig.CarrierSource = owner.Transfers()
			c.networkConfig.CarrierTarget = owner.Transfers()
		}
	}
	c.networkConfig.AttachmentControl = owner.Facets()
	c.networkConfig.Federation = owner.Federation()
	server, err := coregrpc.New(owner.Access(), auth, c.networkConfig)
	if err != nil {
		return err
	}
	defer server.Stop()
	// Validate owners and configuration before opening any listener.
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return errors.New("cannot listen on configured address")
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	if c.MountConnections != nil {
		// One bounded recovery pass, not an autonomous retrying writer. The
		// network owner guard permits Finish/ReadDecision but never refreshes a
		// pending user's authority. Unreachable peers leave work retained.
		recoveryContext, cancelRecovery := context.WithTimeout(ctx, c.networkConfig.RPCDeadline)
		recoveryDone := make(chan struct{})
		go func() {
			defer close(recoveryDone)
			_, _ = owner.Mounts().RecoverMounts(recoveryContext)
		}()
		defer func() { cancelRecovery(); <-recoveryDone }()
	}
	fmt.Println("ucupa-core: ready", c.NextHop)
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
