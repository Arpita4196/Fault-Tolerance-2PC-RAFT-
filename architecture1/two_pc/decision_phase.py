import os
import grpc
from concurrent import futures

import two_phase_pb2 as pb
import two_phase_pb2_grpc as rpc

NODE_ID = os.environ.get("NODE_ID", "service-node1")
DECISION_PORT = int(os.environ.get("DECISION_PORT", "50053"))

prepared = set()


def log_server(phase, rpc_name, from_phase, from_node):
    print(
        f"Phase {phase} of Node {NODE_ID} receives RPC {rpc_name} "
        f"from Phase {from_phase} of Node {from_node}",
        flush=True
    )


def apply_transaction(txn):
    print(f"[{NODE_ID}] APPLYING TRANSACTION {txn.id} -> {txn.payload}", flush=True)


class DecisionServiceServicer(rpc.DecisionServiceServicer):

    def Prepare(self, request, context):
        log_server("Decision", "Prepare", request.from_phase, request.from_node)
        prepared.add(request.txn.id)
        return pb.Ack(ok=True, info=f"{NODE_ID} prepared {request.txn.id}")

    def GlobalCommit(self, request, context):
        log_server("Decision", "GlobalCommit",
                   request.from_phase, request.from_node)

        prepared.add(request.txn.id)
        apply_transaction(request.txn)
        return pb.Ack(ok=True, info=f"{NODE_ID} committed {request.txn.id}")

    def GlobalAbort(self, request, context):
        log_server("Decision", "GlobalAbort",
                   request.from_phase, request.from_node)

        prepared.discard(request.txn.id)
        return pb.Ack(ok=True, info=f"{NODE_ID} aborted {request.txn.id}")


def serve_decision():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    rpc.add_DecisionServiceServicer_to_server(DecisionServiceServicer(), server)
    server.add_insecure_port(f"[::]:{DECISION_PORT}")
    server.start()
    print(f"[{NODE_ID}] Decision phase gRPC on :{DECISION_PORT}", flush=True)
    return server
