package facetgrpc

import (
	"fmt"
	"google.golang.org/protobuf/reflect/protoreflect"
	"ucupa/internal/protovalue"
)

var representation = protovalue.Codec{
	Operations: map[protoreflect.FullName]bool{"ucupa.facet.MutationCommand": true},
	Validate:   validateShape,
}

// These checks concern wire shape only. Acceptance, content revisions,
// authorization and mutation recovery remain the Facet owner's decisions.
func validateShape(m protoreflect.Message) error {
	d := m.Descriptor()
	if f := d.Fields().ByName("type_ref"); f != nil && m.Has(f) && len(m.Get(f).Bytes()) != 32 {
		return fmt.Errorf("invalid TypeRef")
	}
	switch d.FullName() {
	case "ucupa.facet.Expected":
		f := d.Fields().ByName("absent")
		if m.Has(f) && !m.Get(f).Bool() {
			return fmt.Errorf("absent must be true")
		}
	case "ucupa.facet.GetResult":
		state := m.Get(d.Fields().ByName("state")).Enum()
		payload, revision := m.Has(d.Fields().ByName("payload")), m.Has(d.Fields().ByName("revision"))
		if state == 1 && (!payload || !revision) || state == 2 && (payload || revision) {
			return fmt.Errorf("invalid presence")
		}
	case "ucupa.facet.ListTypesResult":
		list := m.Get(d.Fields().ByName("types")).List()
		if list.Len() > 256 {
			return fmt.Errorf("too many types")
		}
		for i := 0; i < list.Len(); i++ {
			if len(list.Get(i).Bytes()) != 32 {
				return fmt.Errorf("invalid digest")
			}
		}
	case "ucupa.facet.ChangeGroup":
		list := m.Get(d.Fields().ByName("category")).List()
		seen := map[protoreflect.EnumNumber]bool{}
		if list.Len() == 0 || list.Len() > 4 {
			return fmt.Errorf("invalid categories")
		}
		for i := 0; i < list.Len(); i++ {
			n := list.Get(i).Enum()
			if seen[n] {
				return fmt.Errorf("duplicate category")
			}
			seen[n] = true
		}
	case "ucupa.facet.PollChangesResult":
		if m.Get(d.Fields().ByName("groups")).List().Len() > 256 {
			return fmt.Errorf("too many groups")
		}
	case "ucupa.facet.ReleaseObserveResult":
		if !m.Get(d.Fields().ByName("released")).Bool() {
			return fmt.Errorf("invalid release")
		}
	}
	return nil
}
