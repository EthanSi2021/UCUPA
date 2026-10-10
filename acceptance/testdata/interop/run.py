"""Runs the same public corpus against Go products and a Python reference."""
import copy
import importlib.metadata
import json
import pathlib
import sys


def check_runtime():
    for line in pathlib.Path(__file__).with_name("requirements.txt").read_text().splitlines():
        if not line or line.startswith("#"):
            continue
        package, required = line.split("==")
        actual = importlib.metadata.version(package)
        if actual != required:
            raise RuntimeError(f"{package}: need {required}, found {actual}")
    if sys.version_info[:2] != (3, 12):
        raise RuntimeError("independent reference requires Python 3.12")


check_runtime()
if sys.argv[1:] == ["--check"]:
    print("independent runtime: PASS")
    raise SystemExit(0)

import grpc
from core_reference import CoreReference
from facet_reference import FacetReference
from wire import Endpoint, Peer, Schema, b64, unb64


def lookup(value, path):
    for part in path.split(".") if path else []:
        value = value[int(part)] if isinstance(value, list) else value[part]
    return value


def expand(value, variables):
    if isinstance(value, str) and value.startswith("$bytes:"):
        return b64(value[7:].encode("utf8"))
    if isinstance(value, str) and value.startswith("$"):
        return copy.deepcopy(lookup(variables, value[1:]))
    if isinstance(value, dict):
        return {k: expand(v, variables) for k, v in value.items()}
    if isinstance(value, list):
        return [expand(v, variables) for v in value]
    return value


def tuple_at(root, x, radius):
    return {"parent": root, "pose": {"translation": {"items": [x, 0, 0]}, "rotation": {"pivot": 4, "ratios": {"items": [0, 0, 0]}}}, "range": {"kind": "BOUNDED_SPHERE", "radius": radius}}


def run_corpus(schema, config, corpus):
    peers = {name: Peer(schema, config[name]) for name in ("core", "facet")}
    root = config["root"]
    variables = {"root": root, "entity": "00000000-0000-4000-8000-000000000002", "other": "00000000-0000-4000-8000-000000000003",
                 "reference": {"boundary": {"core": config["core"]["owner"], "anchor": root}, "next_hop": config["core"]["address"]},
                 "facet_address": config["facet"]["address"], "type_ref": b64(unb64(config["type_ref"])), "large_tuple": tuple_at(root, 3, 2), "far_tuple": tuple_at(root, 10, 1), "new_tuple": tuple_at(root, 4, 2)}
    passed = []
    try:
        for step in corpus:
            name, owner, method = step["name"], step["owner"], step["method"]
            peer = peers[owner]
            if method == "LOGIN":
                peer.login()
                passed.append(name)
                continue
            service = "ucupa.network.SessionService" if step.get("session") else "ucupa." + owner + "." + owner.title() + "Service"
            request = expand(step.get("request", {}), variables)
            kwargs = {"authenticated": step.get("authenticated", True)}
            if step.get("unknown_field"):
                kwargs["suffix"] = b"\x98\x06\x01"
            if step.get("wrong_target"):
                kwargs["target"] = "00000000-0000-4000-8000-000000009999"
            try:
                response = peer.call(service, method, request, **kwargs)
            except grpc.RpcError as error:
                if error.code().name != step.get("status"):
                    raise AssertionError(f"{name}: wanted status {step.get('status')}, got {error.code().name}") from None
                passed.append(name)
                continue
            if step.get("status"):
                raise AssertionError(f"{name}: wanted {step['status']}, got gRPC OK")
            for path, expected in step.get("expect", {}).items():
                try:
                    actual = lookup(response, path)
                except KeyError:
                    raise AssertionError(f"{name}: missing {path}; failure={response.get('failure')}") from None
                if actual != expand(expected, variables):
                    raise AssertionError(f"{name}: mismatch at {path}")
            for path, count in step.get("length", {}).items():
                try:
                    actual = lookup(response, path)
                except KeyError:
                    actual = []
                if len(actual) != count:
                    raise AssertionError(f"{name}: length at {path}: {len(actual)} != {count}")
            for variable, path in step.get("capture", {}).items():
                variables[variable] = copy.deepcopy(lookup(response, path))
            passed.append(name)
    finally:
        for peer in peers.values():
            peer.channel.close()
    return passed


def main():
    config = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf8"))
    corpus = json.loads(pathlib.Path(__file__).with_name("corpus.json").read_text(encoding="utf8"))
    schema = Schema(config["descriptor"])
    reference = copy.deepcopy(config)
    facet = FacetReference(reference["facet"], config["root"], config["core"]["owner"], config["type_ref"])
    facet_endpoint = Endpoint(schema, reference["facet"], facet)
    facet.access["next_hop"] = reference["facet"]["address"]
    core = CoreReference(reference["core"], config["root"], facet)
    core_endpoint = Endpoint(schema, reference["core"], core)
    facet_endpoint.server.start()
    core_endpoint.server.start()
    try:
        python_cases = run_corpus(schema, reference, corpus)
        go_cases = run_corpus(schema, config, corpus)
        if python_cases != go_cases or len(go_cases) != len(corpus):
            raise AssertionError("the two implementations did not execute the same corpus")
        print("INDEPENDENT_CORPUS_PASS " + json.dumps({"cases": len(go_cases), "implementations": ["python-reference", "go-product-core-facet"], "scope": "local-space-attached-content", "federation": "not-proven"}, sort_keys=True))
    finally:
        core_endpoint.server.stop(0).wait(3)
        facet_endpoint.server.stop(0).wait(3)


if __name__ == "__main__":
    main()
