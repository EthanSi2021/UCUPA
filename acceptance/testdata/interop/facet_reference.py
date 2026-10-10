"""Independent reference for one attached namespace and opaque content."""
import copy

from wire import State, b64, canonical, unb64


class FacetReference(State):
    service = "ucupa.facet.FacetService"

    def __init__(self, config, root, core_owner, typ):
        super().__init__(config["owner"])
        self.config, self.typ = config, b64(unb64(typ))
        self.payload, self.revision = None, None
        # Provisioning is completed by the independent Core before either
        # endpoint starts; Facet must not manufacture a Core-issued token.
        self.access = {"next_hop": config["address"], "key": {"core": core_owner, "facet": self.owner, "element": root}}

    def call(self, method, request):
        observed = self.observe(method, request)
        if observed is not None:
            return observed
        if request["access"] != self.access:
            return {"failure": "DENIED"}
        if method == "Get":
            result = {"receipt": self.token("READ_RECEIPT"), "state": "NOT_FOUND", "type_ref": self.typ}
            if self.payload is not None:
                result.update(state="PRESENT", payload=self.payload, revision=self.revision)
            return self.preparation(request, result)
        if method == "ListTypes":
            return self.preparation(request, {"receipt": self.token("READ_RECEIPT"), "types": [] if self.payload is None else [self.typ]})
        if method == "RecoverOutcome":
            command = request["request"]
            name = next(iter(command))
            operation = self.operations.get(request["operation_id"])
            if operation is None:
                return {"failure": "OUTCOME_UNKNOWN"}
            return operation[1] if operation[0] == canonical({"method": name, "request": command[name]}) else {"failure": "CONFLICT"}
        if method in ("Put", "Delete"):
            return self.operation(request["operation_id"], {"method": method.lower(), "request": request}, lambda: self.mutate(method, request))
        raise NotImplementedError(method)

    def mutate(self, method, request):
        if method == "Put":
            expected = request["expected"]
            if (expected.get("absent") is True and self.payload is not None) or ("revision" in expected and expected["revision"] != self.revision):
                return {"failure": "CONFLICT"}
            # The payload is bounded, byte-exact opaque content: no schema
            # validation is performed by either implementation.
            if self.payload == request["payload"]:
                return {"result": {"status": "UNCHANGED", "revision": copy.deepcopy(self.revision)}}
            self.payload = request["payload"]
        else:
            if self.payload is None or request["expected"] != self.revision:
                return {"failure": "CONFLICT"}
            self.payload = None
        self.revision = self.token("RECORD_REVISION")
        self.changed()
        return {"result": {"status": "COMMITTED", "revision": copy.deepcopy(self.revision)}}
