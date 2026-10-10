// Command ucupa-realm serves one existing durable Realm as a pinned network
// service. The -init mode performs the one explicit trusted bootstrap of a
// new Store; it never extends into remote permission management.
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

	realminstance "ucupa/ucupa-realm/instance"
	realmgrpc "ucupa/ucupa-realm/transport/grpc"
)

func main() {
	bootstrap := flag.String("init", "", "trusted bootstrap file for one new Realm Store")
	store := flag.String("store", "", "Realm Store path used with -init")
	path := flag.String("config", "", "service configuration file")
	flag.Parse()
	if flag.NArg() != 0 || (*bootstrap == "" && *path == "") || (*bootstrap != "" && *path != "") ||
		(*bootstrap != "" && *store == "") || (*bootstrap == "" && *store != "") {
		fmt.Fprintln(os.Stderr, "usage: ucupa-realm -config <file>")
		fmt.Fprintln(os.Stderr, "       ucupa-realm -init <bootstrap file> -store <path>")
		os.Exit(2)
	}
	if *bootstrap != "" {
		if err := bootstrapStore(*bootstrap, *store); err != nil {
			fmt.Fprintln(os.Stderr, "ucupa-realm:", err)
			os.Exit(1)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, *path); err != nil {
		fmt.Fprintln(os.Stderr, "ucupa-realm:", err)
		os.Exit(1)
	}
}

// bootstrapStore creates one Realm Store from the explicit trusted document.
// Nothing is seeded beyond it, and the only output is the created identity.
func bootstrapStore(bootstrapPath, storePath string) error {
	seed, err := readBootstrap(bootstrapPath)
	if err != nil {
		return err
	}
	owner, err := realminstance.CreateDurable(seed, storePath)
	if err != nil {
		return errors.New("cannot create Realm Store")
	}
	defer owner.Close()
	fmt.Println("ucupa-realm: created", owner.ID())
	return nil
}

func run(ctx context.Context, path string) error {
	c, err := readConfig(path)
	if err != nil {
		return err
	}
	policy, err := realminstance.OpenDurable(c.RealmStore)
	if err != nil {
		return errors.New("cannot open Realm Store")
	}
	defer policy.Close()
	if policy.ID() != c.RealmID {
		return errors.New("Realm identity mismatch")
	}
	// A present foreign section is assembled completely before serving; an
	// absent one leaves the foreign port fail-closed.
	network, closeForeign, err := c.Foreign.open(c.network)
	if err != nil {
		return err
	}
	defer closeForeign()
	server, err := realmgrpc.New(policy, network)
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
	fmt.Println("ucupa-realm: ready", listener.Addr(), "advertised", c.NextHop)
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
