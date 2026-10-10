package coresdk

import (
	"google.golang.org/protobuf/proto"
	pb "ucupa/ucupa-core/bindings/go/protobuf"
)

func (c *Client) validateFederationObservation(r *pb.ObserveFederationReadPartRequest, out any) error {
	w, ok := out.(*pb.ObserveFederationReadPartResponse)
	if !ok {
		return badFederation()
	}
	if w.GetResult() == nil {
		return nil
	}
	c.mu.RLock()
	part := c.federation
	c.mu.RUnlock()
	page := w.GetResult()
	if part == nil || part.Preparation == nil || !proto.Equal(part.Basis, r.Basis) || page.Cursor == nil {
		return badFederation()
	}
	if part.GetHidden() && len(page.Groups) != 0 {
		return badFederation()
	}
	if r.GetOperation() == "begin" {
		if r.Cursor != nil || r.Limit != nil || len(page.Groups) != 0 {
			return badFederation()
		}
	} else if r.GetOperation() == "poll" {
		if r.Cursor == nil || r.GetLimit() == 0 || r.GetLimit() > 256 || uint64(len(page.Groups)) > uint64(r.GetLimit()) {
			return badFederation()
		}
	} else {
		return badFederation()
	}
	seen := map[string]bool{}
	for _, g := range page.Groups {
		if g == nil || g.GetOwner() != string(c.config.Expected) || g.ChangeId == nil || g.Basis == nil || seen[string(g.ChangeId.Value)] || len(g.Category) == 0 || len(g.Category) > 2 {
			return badFederation()
		}
		seen[string(g.ChangeId.Value)] = true
		spatial, attachment := false, false
		for _, category := range g.Category {
			switch category {
			case pb.Category_SPATIAL:
				if spatial {
					return badFederation()
				}
				spatial = true
			case pb.Category_ATTACHMENT:
				if attachment {
					return badFederation()
				}
				attachment = true
			default:
				return badFederation()
			}
		}
	}
	return nil
}
