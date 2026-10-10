package coordinator

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"
	accessresolve "ucupa/ucupa-core/internal/access/resolve"
	federationsource "ucupa/ucupa-core/internal/query/federation/source"

	"ucupa/shared/values"
	corecontract "ucupa/ucupa-core/contract"
	federationcontract "ucupa/ucupa-core/contract/federation"
	accessregistry "ucupa/ucupa-core/internal/access/registry"
	accessvalue "ucupa/ucupa-core/internal/access/value"
	federationcollect "ucupa/ucupa-core/internal/query/federation/collect"
)

func retireFederatedRead(read *federationcollect.Read) {
	if read == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = read.Retire(ctx)
}

func retireFederatedConnection(connection federationcontract.FederationReadConnection) {
	if connection == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = connection.Release(ctx)
}

func (core *Operations) Read(ctx context.Context, caller values.CallerContext, operation string, request map[string]any, connections federationcontract.FederationConnections) (map[string]any, error) {
	// Projection decoding and this invocation's output merge are transient.
	const working uint64 = 16 << 20
	core.owner.Lock()
	workingReservation, reserveErr := core.registry.ReserveLocked(working)
	core.owner.Unlock()
	if reserveErr != nil {
		return nil, reserveErr
	}
	defer core.registry.Release(workingReservation)
	allowed := []string{"op", "handle", "prepareObserve"}
	switch operation {
	case "read":
		allowed = append(allowed, "address", "basis")
	case "list":
		allowed = append(allowed, "address", "limit", "continuation")
	case "query":
		allowed = append(allowed, "start", "basis", "upDepth", "sphere", "selector", "limit", "continuation")
	}
	if _, err := accessvalue.Object(request, allowed...); err != nil {
		return nil, err
	}
	prepare := false
	if raw, exists := request["prepareObserve"]; exists {
		var valid bool
		prepare, valid = raw.(bool)
		if !valid {
			return nil, values.PublicInvalid
		}
	}
	if prepare && operation == "read" && request["basis"] != nil {
		return nil, values.PublicInvalid
	}
	if operation == "read" && request["basis"] != nil {
		return core.readAt(ctx, caller, request)
	}
	limit, err := accessvalue.Limit(request["limit"], 64)
	if err != nil {
		return nil, err
	}
	conditions := accessvalue.ReadConditions(request)
	core.owner.Lock()
	handle, err := accessresolve.SpaceLocked(core.owner, core.registry, caller, request["handle"])
	var receipt accessregistry.ReadUse
	var after values.ElementID
	if err == nil && request["continuation"] != nil {
		var cursor accessregistry.ContinuationUse
		cursor, err = accessresolve.ContinuationLocked(core.owner, core.registry, caller, request["continuation"])
		if err == nil {
			var present bool
			receipt, present = core.registry.ReadLocked(cursor.Receipt)
			if !present || receipt.Composite == nil || receipt.Parent != handle.Ref || receipt.Conditions != conditions {
				err = values.PublicInvalidContext
			} else {
				after = cursor.Last
			}
		}
	}
	var read *federationcollect.Read
	if err == nil && receipt.Ref != (accessregistry.Ref{}) {
		read = receipt.Composite
	}
	core.owner.Unlock()
	if err != nil {
		return nil, err
	}
	initial := receipt.Ref == (accessregistry.Ref{})
	if initial {
		core.owner.Lock()
		receipt, err = core.registry.IssueCompositeReadLocked(caller, handle.Boundary, handle.Ref, operation, conditions)
		core.owner.Unlock()
		if err != nil {
			if receipt.Ref != (accessregistry.Ref{}) {
				core.owner.Lock()
				core.registry.RemoveLocked(receipt.Ref)
				core.owner.Unlock()
			}
			return nil, err
		}
	}
	// Receipt expiry cancels active source I/O before retirement waits for the
	// collector mutex.
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(receipt.Lifetime, cancel)
	defer func() { stop(); cancel() }()
	ctx = callCtx
	keep := false
	defer func() {
		if initial && !keep {
			core.owner.Lock()
			core.registry.RemoveLocked(receipt.Ref)
			core.owner.Unlock()
			retireFederatedRead(read)
		}
	}()
	if initial {
		partRequest := federationcontract.FederationReadRequest{
			Boundary:       handle.Boundary,
			Intent:         operation,
			PrepareObserve: prepare,
			Purpose:        sha256.Sum256([]byte(conditions)),
		}
		for attempt := 0; attempt < 2; attempt++ {
			read, err = core.buildFederatedRead(ctx, caller, partRequest, request, connections)
			if !errors.Is(err, federationcollect.ErrMixedRetention) {
				break
			}
			partRequest.Mode = corecontract.QueryCurrentVerified
		}
		if err != nil {
			return nil, err
		}
		core.owner.Lock()
		if current, present := core.registry.ReadLocked(receipt.Ref); !present || current.Ref != receipt.Ref {
			err = values.PublicInvalidContext
		} else if !core.registry.InstallCompositeLocked(receipt.Ref, read) {
			err = values.PublicInvalidContext
		} else {
			receipt, _ = core.registry.ReadLocked(receipt.Ref)
		}
		core.owner.Unlock()
		if err != nil {
			return nil, err
		}
	}
	page, lease, err := read.Collect(ctx, after, limit, prepare)
	if err != nil {
		return nil, err
	}
	defer lease.Release()
	result := federatedPageJSON(operation, handle.Boundary, page)
	result["closure"] = values.EncodePublicBytes(page.Closure[:])
	result["sourceGraph"] = federatedSourceGraphJSON(page.SourceGraph)
	core.owner.Lock()
	defer core.owner.Unlock()
	current, present := core.registry.ReadLocked(receipt.Ref)
	if ctx.Err() != nil || core.owner.ClosedLocked() || !present || current.Composite != read {
		return nil, values.PublicInvalidContext
	}
	if _, err := accessresolve.SpaceLocked(core.owner, core.registry, caller, request["handle"]); err != nil {
		return nil, err
	}
	result["receipt"] = accessvalue.TokenJSON(receipt.Token)
	if prepare {
		if !current.HasPreparation {
			if _, err := core.registry.PrepareCompositeLocked(receipt.Ref); err != nil {
				return nil, err
			}
			current, _ = core.registry.ReadLocked(receipt.Ref)
		}
		prep, ok := core.registry.PreparationLocked(current.Preparation)
		if !ok {
			return nil, values.PublicGap
		}
		result["preparation"] = accessvalue.TokenJSON(prep.Token)
	}
	result["readMode"] = "CURRENT_VERIFIED"
	if page.ReadMode == corecontract.QueryRetained {
		result["readMode"] = "RETAINED"
	}
	if page.Last.Valid() {
		cursor, err := core.registry.IssueContinuationLocked(receipt.Ref, page.Last, "")
		if err != nil {
			return nil, err
		}
		result["continuation"] = accessvalue.TokenJSON(cursor.Token)
	}
	keep = true
	return result, nil
}

func (core *Operations) buildFederatedRead(ctx context.Context, caller values.CallerContext, partRequest federationcontract.FederationReadRequest, request map[string]any, connections federationcontract.FederationConnections) (*federationcollect.Read, error) {
	local, err := federationsource.OpenLocal(core.sources, ctx, caller, partRequest)
	if err != nil {
		return nil, err
	}
	typed, err := decodeFederatedCollectRequest(partRequest.Intent, partRequest.Boundary, request)
	if err != nil {
		retireFederatedConnection(local)
		return nil, err
	}
	return federationcollect.New(ctx, federationcollect.Config{
		Caller:      caller,
		Source:      core.owner.AuthorityRef(),
		PartRequest: partRequest,
		Local:       local,
		Connections: connections,
		SourceLimit: 256,
		Request:     typed,
	})
}
