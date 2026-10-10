package values

import "testing"

func TestNextHopNetworkProfile(t *testing.T) {
	for _, input := range []NextHop{"https://localhost:443", "https://core.example:443", "https://127.0.0.1:1", "https://[::1]:65535", "https://[2001:db8::1]:50051"} {
		if target, ok := input.NetworkTarget(); !ok || target == "" || !input.Valid() || input.IsLocal() {
			t.Errorf("rejected network address %q", input)
		}
	}
	for _, input := range []NextHop{"https://core", "https://core:0", "https://core:65536", "https://core:0443", "https://Core:443", "https://core.:443", "https://user@core:443", "https://core:443/", "https://core:443?", "https://core:443#", "https://core:443/path", "https://127.000.0.1:443", "https://127.1:443", "https://[0:0:0:0:0:0:0:1]:443", "https://[::1%25lo]:443", "https://a..b:443", "https://-a:443", "https://é:443", "https://core:443\n", "http://core:443", "local:core"} {
		if _, ok := input.NetworkTarget(); ok {
			t.Errorf("accepted non-network/noncanonical address %q", input)
		}
		if input != "local:core" && input.Valid() {
			t.Errorf("accepted invalid NextHop %q", input)
		}
	}
}
