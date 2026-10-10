"""Independent Python gRPC/Protobuf boundary; no UCUPA Go import or bridge."""
import base64
import hashlib
import hmac
import json
import pathlib
import secrets
import threading
import time
from concurrent.futures import ThreadPoolExecutor

import grpc
from cryptography import x509
from cryptography.hazmat.primitives.serialization import Encoding, PublicFormat
from google.protobuf import descriptor_pb2, descriptor_pool, json_format, message_factory


def b64(value):
    return base64.b64encode(value).decode("ascii")


def unb64(value):
    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


class Schema:
    def __init__(self, filename):
        bundle = descriptor_pb2.FileDescriptorSet.FromString(pathlib.Path(filename).read_bytes())
        self.pool = descriptor_pool.DescriptorPool()
        for source in bundle.file:
            self.pool.Add(source)
        self.required = self.pool.FindExtensionByName("ucupa.network.required_value")
        self.options = message_factory.GetMessageClass(self.pool.FindMessageTypeByName("google.protobuf.FieldOptions"))

    def message(self, descriptor):
        return message_factory.GetMessageClass(descriptor)

    def method(self, service, name):
        return self.pool.FindServiceByName(service).methods_by_name[name]

    def encode(self, descriptor, value):
        return json_format.ParseDict(value, self.message(descriptor)()).SerializeToString()

    def decode(self, descriptor, raw):
        message = self.message(descriptor).FromString(raw)
        clean = self.message(descriptor)()
        clean.CopyFrom(message)
        clean.DiscardUnknownFields()
        if clean.SerializeToString(deterministic=True) != message.SerializeToString(deterministic=True):
            raise ValueError("unknown protobuf field")
        self.validate(message)
        return json_format.MessageToDict(message, preserving_proto_field_name=True)

    def validate(self, message):
        for field in message.DESCRIPTOR.fields:
            option = self.options.FromString(field.GetOptions().SerializeToString())
            if option.Extensions[self.required] and not message.HasField(field.name):
                raise ValueError("missing required field")
        for field, value in message.ListFields():
            values = value if field.is_repeated else [value]
            if field.type == field.TYPE_MESSAGE:
                for item in values:
                    self.validate(item)
            if field.type == field.TYPE_ENUM:
                for item in values:
                    if item not in field.enum_type.values_by_number:
                        raise ValueError("unknown enum")


class Peer:
    """Exact configured certificate trust, no proxy, redirect or business retry."""
    def __init__(self, schema, config):
        self.schema, self.config, self.session = schema, config, None
        cert = pathlib.Path(config["cert"]).read_bytes()
        leaf = x509.load_pem_x509_certificate(cert)
        spki = leaf.public_key().public_bytes(Encoding.DER, PublicFormat.SubjectPublicKeyInfo)
        if not hmac.compare_digest(hashlib.sha256(spki).digest(), unb64(config["pin"])):
            raise ValueError("configured certificate does not match expected owner key")
        self.channel = grpc.secure_channel(config["address"].removeprefix("https://"), grpc.ssl_channel_credentials(root_certificates=cert), options=[
            ("grpc.enable_retries", 0), ("grpc.enable_http_proxy", 0),
            ("grpc.max_receive_message_length", 8 << 20), ("grpc.max_send_message_length", 4 << 20),
            ("grpc.max_metadata_size", 8192),
        ])

    def call(self, service, name, request, *, authenticated=True, suffix=b"", target=None):
        method = self.schema.method(service, name)
        headers = [("ucupa-target", target or self.config["owner"])]
        if authenticated and self.session:
            headers.append(("authorization", "Bearer " + self.session))
        rpc = self.channel.unary_unary("/" + service + "/" + name)
        raw = rpc(self.schema.encode(method.input_type, request) + suffix, metadata=headers, timeout=5, wait_for_ready=False)
        return self.schema.decode(method.output_type, raw)

    def login(self):
        cfg = self.config
        value = self.call("ucupa.network.SessionService", "EstablishSession", {
            "audience": cfg["audience"], "credential_id": b64(unb64(cfg["id"])), "secret": b64(unb64(cfg["secret"]))
        }, authenticated=False)
        if len(unb64(value["session"])) != 32 or int(value["expires_in_seconds"]) <= 0:
            raise AssertionError("invalid session reply")
        self.session = base64.urlsafe_b64encode(unb64(value["session"])).rstrip(b"=").decode("ascii")


class Endpoint:
    def __init__(self, schema, config, engine):
        self.schema, self.config, self.engine = schema, config, engine
        self.sessions, self.lock = {}, threading.RLock()
        self.server = grpc.server(ThreadPoolExecutor(max_workers=4), maximum_concurrent_rpcs=4, options=[
            ("grpc.max_receive_message_length", 4 << 20), ("grpc.max_send_message_length", 8 << 20),
            ("grpc.max_metadata_size", 8192), ("grpc.max_concurrent_streams", 8),
        ])
        for service in ("ucupa.network.SessionService", engine.service):
            descriptor = schema.pool.FindServiceByName(service)
            handlers = {}
            for method in descriptor.methods:
                handlers[method.name] = grpc.unary_unary_rpc_method_handler(self.handler(service, method))
            self.server.add_generic_rpc_handlers((grpc.method_handlers_generic_handler(service, handlers),))
        cert, key = pathlib.Path(config["cert"]).read_bytes(), pathlib.Path(config["key"]).read_bytes()
        port = self.server.add_secure_port("127.0.0.1:0", grpc.ssl_server_credentials([(key, cert)]))
        if not port:
            raise RuntimeError("reference endpoint failed to bind")
        config["address"] = "https://127.0.0.1:" + str(port)

    def handler(self, service, method):
        def call(raw, context):
            with self.lock:
                try:
                    request = self.schema.decode(method.input_type, raw)
                except (ValueError, TypeError):
                    context.abort(grpc.StatusCode.INVALID_ARGUMENT, "invalid message")
                headers = context.invocation_metadata()
                targets = [v for k, v in headers if k == "ucupa-target"]
                if len(targets) != 1:
                    context.abort(grpc.StatusCode.INVALID_ARGUMENT, "invalid target metadata")
                if targets[0] != self.config["owner"]:
                    context.abort(grpc.StatusCode.UNAVAILABLE, "target mismatch")
                authorization = [v for k, v in headers if k == "authorization"]
                if service == "ucupa.network.SessionService" and method.name == "EstablishSession":
                    cfg = self.config
                    if request != {"audience": cfg["audience"], "credential_id": b64(unb64(cfg["id"])), "secret": b64(unb64(cfg["secret"]))}:
                        context.abort(grpc.StatusCode.UNAUTHENTICATED, "invalid credential")
                    if len(self.sessions) >= 8:
                        context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "session capacity")
                    session = secrets.token_bytes(32)
                    bearer = base64.urlsafe_b64encode(session).rstrip(b"=").decode("ascii")
                    self.sessions["Bearer " + bearer] = time.monotonic() + 30
                    result = {"session": b64(session), "expires_in_seconds": "30"}
                elif service == "ucupa.network.SessionService" and method.name == "ReleaseSession":
                    if len(authorization) != 1:
                        context.abort(grpc.StatusCode.UNAUTHENTICATED, "session required")
                    self.sessions.pop(authorization[0], None)
                    result = {}
                else:
                    if len(authorization) != 1 or self.sessions.get(authorization[0], 0) <= time.monotonic():
                        context.abort(grpc.StatusCode.UNAUTHENTICATED, "session required")
                    self.engine.session = authorization[0]
                    try:
                        result = self.engine.call(method.name, request)
                    except ValueError:
                        context.abort(grpc.StatusCode.INVALID_ARGUMENT, "invalid request")
                    except NotImplementedError:
                        context.abort(grpc.StatusCode.UNIMPLEMENTED, "outside reference corpus")
                return self.schema.encode(method.output_type, result)
        return call


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


class State:
    def __init__(self, owner):
        self.owner, self.session, self.serial = owner, None, 0
        self.tokens, self.operations, self.changes = {}, {}, []

    def token(self, kind, value=None):
        token = {"owner": self.owner, "kind": kind, "value": b64(secrets.token_bytes(32))}
        self.tokens[token["value"]] = (kind, self.session, value)
        return token

    def context(self, token, kind):
        item = self.tokens.get(token.get("value"))
        if token.get("owner") != self.owner or token.get("kind") != kind or item is None or item[:2] != (kind, self.session):
            return None
        return item[2]

    def changed(self):
        self.serial += 1
        self.changes.append({"change_id": self.token("CHANGE_ID"), "owner": self.owner,
                             "category": ["SPATIAL" if self.service == "ucupa.core.CoreService" else "CONTENT"],
                             "basis": self.token("CHANGE_BASIS")})

    def observe(self, method, request):
        if method == "BeginObserve":
            cut = self.context(request["preparation"], "OBSERVATION_PREPARATION")
            if cut is None:
                return {"failure": "DENIED"}
            return {"result": {"cursor": self.token("OBSERVATION_CURSOR", cut)}}
        if method == "PollChanges":
            cut = self.context(request["cursor"], "OBSERVATION_CURSOR")
            if cut is None:
                return {"failure": "DENIED"}
            groups = self.changes[cut:cut + request.get("limit", 100)]
            return {"result": {"groups": groups, "next_cursor": self.token("OBSERVATION_CURSOR", cut + len(groups))}}
        if method == "ReleaseObserve":
            token = request.get("cursor", request.get("preparation", {}))
            self.tokens.pop(token.get("value"), None)
            return {"result": {"released": True}}
        return None

    def preparation(self, request, result):
        if request.get("prepare_observe", False):
            result["preparation"] = self.token("OBSERVATION_PREPARATION", len(self.changes))
        return {"result": result}

    def operation(self, operation_id, command, execute):
        binding = canonical(command)
        if operation_id in self.operations:
            old, result = self.operations[operation_id]
            return result if binding == old else {"failure": "CONFLICT"}
        result = execute()
        self.operations[operation_id] = (binding, result)
        return result
