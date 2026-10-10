"""Test-only independent local spatial semantics for the declared corpus.

No storage, authorization, transport or gateway code is reused from Go.
The fixture grants all declared intents under its one root. This engine is not
a federation implementation, a Realm replacement, or a production Core.
"""
import copy
import math

from wire import State, canonical


class CoreReference(State):
    service = "ucupa.core.CoreService"

    def __init__(self, config, root, facet):
        super().__init__(config["owner"])
        self.config, self.root, self.facet = config, root, facet
        facet.access["key"]["proposal"] = self.token("ATTACHMENT_REVISION")
        self.records = {root: {"kind": "ANCHOR", "element": root, "revision": self.token("RECORD_REVISION"),
                               "range": {"kind": "UNBOUNDED_ROOT"}}}
        self.attachments = {root: {"state": "ACTIVE", "revision": self.token("ATTACHMENT_REVISION"),
                                  "key": facet.access["key"], "next_hop": facet.config["address"],
                                  "contact_revision": self.token("CONTACT_REVISION")}}

    def address(self, element):
        path = []
        while element != self.root:
            path.append(element)
            element = self.records[element]["parent"]
        return {"down": {"items": list(reversed(path))}} if path else {}

    def locate(self, address):
        element = address.get("origin", self.root)
        if element not in self.records:
            return None
        for _ in range(int(address.get("up", 0))):
            element = self.records[element].get("parent")
            if element is None:
                return None
        for child in address.get("down", {}).get("items", []):
            if self.records.get(child, {}).get("parent") != element:
                return None
            element = child
        return element

    def positioned(self, element):
        result = [0.0, 0.0, 0.0]
        while element != self.root:
            record = self.records[element]
            result = [a + b for a, b in zip(result, record["pose"]["translation"]["items"])]
            element = record["parent"]
        return result

    def public_record(self, record):
        result = copy.deepcopy(record)
        result["address"] = self.address(record["element"])
        return result

    def call(self, method, request):
        observed = self.observe(method, request)
        if observed is not None:
            return observed
        if method == "Enter":
            if request["reference"]["boundary"] != {"core": self.owner, "anchor": self.root} or request["reference"]["next_hop"] != self.config["address"]:
                return {"failure": "DENIED"}
            return {"result": {"handle": self.token("SPACE_HANDLE", True), "boundary": request["reference"]["boundary"]}}
        if method == "Close":
            self.tokens.pop(request["handle"]["value"], None)
            return {"result": {"closed": True}}
        if method == "RecoverOutcome":
            command = request["request"]
            key = next(iter(command))
            operation = self.operations.get(request["operation_id"])
            if operation is None:
                return {"failure": "OUTCOME_UNKNOWN"}
            return operation[1] if operation[0] == canonical({"method": key, "request": command[key]}) else {"failure": "CONFLICT"}
        if self.context(request["handle"], "SPACE_HANDLE") is None:
            return {"failure": "DENIED"}
        if method == "ReadElement":
            return self.read(request)
        if method == "ListDirect":
            element = self.locate(request["address"])
            records = [self.public_record(r) for r in self.records.values() if r.get("parent") == element]
            records.sort(key=lambda item: item["element"])
            return self.page(request, records, "records")
        if method == "QueryRegion":
            return self.query(request)
        if method.startswith(("Create", "Update", "Delete")):
            original = {k: v for k, v in request.items() if k != "handle"}
            name = "".join("_" + c.lower() if c.isupper() else c for c in method).lstrip("_")
            return self.operation(request["operation_id"], {"method": name, "request": original}, lambda: self.mutate(method, request))
        raise NotImplementedError(method)

    def read(self, request):
        element = self.locate(request["address"])
        if "basis" in request:
            basis = self.context(request["basis"], "READ_RECEIPT")
            if basis is None or basis != self.serial:
                return {"failure": "GAP"}
        result = {"receipt": self.token("READ_RECEIPT", self.serial), "read_mode": "CURRENT_VERIFIED", "state": "NOT_FOUND"}
        if element:
            result.update(state="PRESENT", record=self.public_record(self.records[element]), attachment=self.attachments[element])
        return self.preparation(request, result)

    def page(self, request, items, field, extra=None):
        offset = 0
        signature = canonical({k: v for k, v in request.items() if k not in ("continuation", "handle")})
        if "continuation" in request:
            previous = self.context(request["continuation"], "QUERY_CONTINUATION")
            if previous is None or previous[0] != self.serial or previous[1] != signature:
                return {"failure": "GAP"}
            offset = previous[2]
        limit = request.get("limit", 100)
        if limit <= 0:
            return {"failure": "INVALID"}
        result = {"receipt": self.token("READ_RECEIPT", self.serial), "read_mode": "CURRENT_VERIFIED", "status": "COMPLETE", field: items[offset:offset + limit]}
        if offset + limit < len(items):
            result["continuation"] = self.token("QUERY_CONTINUATION", (self.serial, signature, offset + limit))
        if extra:
            result.update(extra)
        return self.preparation(request, result)

    def query(self, request):
        scope = self.locate(request["start"])
        basis = self.locate(request["basis"])
        depth = int(request.get("up_depth", 0))
        if depth < -1:
            raise ValueError("invalid upward depth")
        if scope is None or basis is None:
            return {"failure": "INVALID"}
        actual = 0
        while scope != self.root and (depth == -1 or actual < depth):
            scope = self.records[scope]["parent"]
            actual += 1
        center = [a + b for a, b in zip(self.positioned(basis), request["sphere"]["center"]["items"])]
        radius = request["sphere"]["radius"]
        if radius < 0 or not all(math.isfinite(v) for v in center + [radius]):
            return {"failure": "INVALID"}
        descendants, found = {scope}, []
        while True:
            expanded = descendants | {k for k, r in self.records.items() if r.get("parent") in descendants}
            if expanded == descendants:
                break
            descendants = expanded
        for element in sorted(descendants):
            record = self.records[element]
            if request.get("selector", "BOTH") not in ("BOTH", "ANCHORS" if record["kind"] == "ANCHOR" else "ENTITIES"):
                continue
            if element == self.root or math.dist(center, self.positioned(element)) <= radius + record["range"]["radius"]:
                found.append({"element": element, "kind": record["kind"], "revision": record["revision"], "address": self.address(element)})
        return self.page(request, found, "matches", {"scope": scope, "actual_up_depth": str(actual)})

    def mutate(self, method, request):
        element, old = request["element"], self.records.get(request["element"])
        kind = "ANCHOR" if method.endswith("Anchor") else "ENTITY"
        if method.startswith("Create"):
            if old:
                return {"failure": "CONFLICT"}
        elif old is None or old["revision"] != request["expected"]:
            return {"failure": "CONFLICT"}
        if old and (element == self.root or old["kind"] != kind):
            return {"failure": "INVALID"}
        if method.startswith("Delete"):
            if any(r.get("parent") == element for r in self.records.values()):
                return {"failure": "INVALID"}
            del self.records[element]
            self.changed()
            return {"result": {"status": "COMMITTED", "tombstone_revision": self.token("RECORD_REVISION")}}
        final = copy.deepcopy(request["final"])
        parent = self.records.get(final["parent"])
        pose = final["pose"]
        translation = pose["translation"]["items"]
        radius = final["range"].get("radius", -1)
        if parent is None or parent["kind"] != "ANCHOR" or len(translation) != 3 or radius < 0 or not all(math.isfinite(v) for v in translation + [radius]):
            return {"failure": "INVALID"}
        # The published corpus only uses canonical identity rotation. Do not
        # silently provide incorrect results for an unimplemented rotation.
        if pose["rotation"] != {"pivot": 4, "ratios": {"items": [0.0, 0.0, 0.0]}}:
            raise NotImplementedError("reference rotation outside corpus")
        if final["parent"] != self.root and math.dist(translation, [0, 0, 0]) + radius > parent["range"]["radius"]:
            return {"failure": "INVALID"}
        if old and {k: old[k] for k in ("parent", "pose", "range")} == final:
            return {"result": {"status": "UNCHANGED", "record": self.public_record(old)}}
        record = dict(final, kind=kind, element=element, revision=self.token("RECORD_REVISION"))
        self.records[element] = record
        self.attachments.setdefault(element, {"state": "NONE", "revision": self.token("ATTACHMENT_REVISION")})
        self.changed()
        return {"result": {"status": "COMMITTED", "record": self.public_record(record)}}
