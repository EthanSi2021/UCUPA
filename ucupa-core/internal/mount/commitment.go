package mount

import (
	"crypto/sha256"
	"strconv"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	commitpkg "ucupa/ucupa-core/internal/commit"
	commitbinding "ucupa/ucupa-core/internal/commit/binding"
	coretoken "ucupa/ucupa-core/internal/token"
)

// CommandCommitment commits to a canonical intent before a parent reserves
// any control slot. It does not issue a revision, grant permission or read facts.
func (core *Operations) CommandCommitment(parent, carrier values.FactAuthorityRef, id corecontract.OperationID, command corecontract.MountCommand) ([32]byte, error) {
	c, ok := commitpkg.CanonicalMountCommand(command)
	if !ok || !parent.Valid() || !carrier.Valid() || parent == carrier || !commitbinding.ValidOperationID(id) || c.Kind == corecontract.MountRevoke || c.Kind == corecontract.MountEstablish && c.CarrierAuthority != carrier {
		return [32]byte{}, corecontract.ErrInvalid
	}
	revision := func(scope [32]byte, element values.ElementID, serial uint64) any {
		if serial == 0 {
			return nil
		}
		return []any{values.EncodePublicBytes(scope[:]), string(element), strconv.FormatUint(serial, 10)}
	}
	value := map[string]any{
		"parentCore": string(parent), "carrierCore": string(carrier), "operationId": values.EncodePublicBytes([]byte(id)),
		"element": string(c.MountAnchorUUID), "kind": "establish", "carrierNextHop": string(c.CarrierNextHop), "parentNextHop": string(c.ParentNextHop),
		"expectedElement": revision(coretoken.RevisionScope(c.ExpectedElementRevision), coretoken.RevisionElement(c.ExpectedElementRevision), coretoken.RevisionSerial(c.ExpectedElementRevision)),
		"expectedMount":   revision(coretoken.MountRevisionScope(c.ExpectedMountRevision), coretoken.MountRevisionElement(c.ExpectedMountRevision), coretoken.MountRevisionSerial(c.ExpectedMountRevision)), "final": nil,
	}
	if c.Kind == corecontract.MountUpdate {
		value["kind"] = "update"
		value["final"] = map[string]any{"parent": string(c.Final.Parent), "pose": accessvalue.PoseJSON(c.Final.LocalPose), "radius": c.Final.Range.Radius()}
	}
	raw, err := values.CanonicalJSONValue(value)
	if err != nil {
		return [32]byte{}, corecontract.ErrInvalid
	}
	return sha256.Sum256(append([]byte("ucupa.mount-command\x00"), raw...)), nil
}
