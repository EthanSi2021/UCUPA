// Command protogen regenerates the declared network bindings or checks them in
// a temporary directory. It never downloads tools or edits bindings by hand.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var outputs = []string{"shared/bindings/go/protobuf/network.pb.go", "shared/bindings/go/protobuf/network_grpc.pb.go", "ucupa-core/bindings/go/protobuf/core.pb.go", "ucupa-core/bindings/go/protobuf/core_grpc.pb.go", "ucupa-facet/bindings/go/protobuf/facet.pb.go", "ucupa-facet/bindings/go/protobuf/facet_grpc.pb.go", "ucupa-realm/bindings/go/protobuf/realm.pb.go", "ucupa-realm/bindings/go/protobuf/realm_grpc.pb.go"}

func main() {
	check := flag.Bool("check", false, "compare temporary regenerated bindings without writes")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments")
		os.Exit(2)
	}
	if err := generate(*check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(check bool) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "shared/api/network.proto")); err != nil {
		return fmt.Errorf("run from repository root: %w", err)
	}
	protoc, err := exec.LookPath("protoc")
	if err != nil {
		return fmt.Errorf("protoc 34.1 is required: %w", err)
	}
	if err := version(protoc, "--version", "libprotoc 34.1"); err != nil {
		return err
	}
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goTool += ".exe"
	}
	gp, err := exec.Command(goTool, "env", "GOPATH").Output()
	if err != nil {
		return err
	}
	bin := filepath.Join(strings.Split(strings.TrimSpace(string(gp)), string(os.PathListSeparator))[0], "bin")
	find := func(name, want string) (string, error) {
		p, e := exec.LookPath(name)
		if e != nil {
			p = filepath.Join(bin, name)
			if runtime.GOOS == "windows" {
				p += ".exe"
			}
		}
		if e := version(p, "--version", want); e != nil {
			return "", e
		}
		return p, nil
	}
	goPlugin, err := find("protoc-gen-go", "protoc-gen-go v1.36.12")
	if err != nil {
		return err
	}
	grpcPlugin, err := find("protoc-gen-go-grpc", "protoc-gen-go-grpc 1.6.2")
	if err != nil {
		return err
	}
	out := root
	if check {
		out, err = os.MkdirTemp("", "ucupa-protogen-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(out)
	}
	cmd := exec.Command(protoc, "--plugin=protoc-gen-go="+goPlugin, "--plugin=protoc-gen-go-grpc="+grpcPlugin, "--go_out="+out, "--go_opt=module=ucupa", "--go-grpc_out="+out, "--go-grpc_opt=module=ucupa", "shared/api/network.proto", "ucupa-core/api/core.proto", "ucupa-facet/api/facet.proto", "ucupa-realm/api/realm.proto")
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if check {
		for _, p := range outputs {
			a, e := os.ReadFile(filepath.Join(root, p))
			if e != nil {
				return e
			}
			b, e := os.ReadFile(filepath.Join(out, p))
			if e != nil {
				return e
			}
			if !bytes.Equal(a, b) {
				return fmt.Errorf("generated binding differs: %s", p)
			}
		}
	}
	fmt.Println("Pinned Protobuf generation: PASS")
	return nil
}
func version(program, arg, want string) error {
	out, err := exec.Command(program, arg).CombinedOutput()
	actual := strings.ReplaceAll(strings.TrimSpace(string(out)), ".exe ", " ")
	if err != nil || actual != want {
		return fmt.Errorf("expected %s; tool %s returned %q (%v)", want, program, strings.TrimSpace(string(out)), err)
	}
	return nil
}
