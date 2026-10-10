package values

import (
	"strings"
	"testing"
)

func TestNextHopLocalProfile(t *testing.T) {
	valid := []NextHop{
		"local:a",
		"local:0",
		"local:a-",
		"local:core-b",
		"local:abcdefghijklmnopqrstuvwxyz0123456789-",
		NextHop("local:" + strings.Repeat("a", 63)),
		NextHop("local:0" + strings.Repeat("-", 62)),
	}
	for _, value := range valid {
		t.Run(string(value), func(t *testing.T) {
			if !value.Valid() {
				t.Fatalf("valid NextHop rejected: %q", value)
			}
		})
	}
	invalid := []NextHop{
		"",
		"local:",
		"local:-",
		"local:-a",
		"local:A",
		"local:coreB",
		"LOCAL:core-b",
		"Local:core-b",
		"core-b",
		"local://core-b",
		"local:core-b/",
		"local:core-b:80",
		"local:core.b",
		"local:core_b",
		"local:%61",
		"local:core-b?alias=a",
		"local:core-b#alias",
		"http://core-b",
		" local:core-b",
		"local:core-b ",
		"local:core b",
		"local:core-b\n",
		"local:\tcore-b",
		"local:core\x00b",
		"local:é",
		"local:ａ",
		NextHop("local:" + strings.Repeat("a", 64)),
	}
	for _, value := range invalid {
		t.Run(string(value), func(t *testing.T) {
			if value.Valid() {
				t.Fatalf("invalid or noncanonical NextHop accepted: %q", value)
			}
		})
	}
}
