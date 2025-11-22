import os
import grpc
from concurrent import futures

import two_phase_pb2 as pb
import two_phase_pb2_grpc as rpc

NODE_ID = os.environ.get("NODE_ID", "service-node1")
VOTE_PORT = int(os.environ.get("VOTE_PORT", "50052"))
DECISION_PORT = int(os.environ.get("DECISION_PORT", "50053"))

ABORT_SELF = os.environ.get("ABORT_SELF", "false").lower() == "true"


# -------- Logging Helpers --------
def log_server(phase, rpc_name, from_phase, from_node):
    print(
        f"Phase {phase} of Node {NODE_ID} receives RPC {rpc_name} "
        f"from Phase {from_phase} of Node {from_node}",
        flush=True,
    )


def log_client(from_phase, rpc_name, to_phase, to_node):
    print(
        f"Phase {from_phase} of Node {NODE_ID} sends RPC {rpc_name} "
        f"to Phase {to_phase} of Node {to_node}",
        flush=True,
    )


# -------- Vote Phase Server --------
class VoteServiceServicer(rpc.VoteServiceServicer):

    def VoteRequest(self, request, context):
        log_server("Vote", "VoteRequest",
                   request.from_phase, request.from_node)

        # Decide commit or abort
        vote_commit = not ABORT_SELF
        reason = "Prepared" if vote_commit else "ABORT_SELF=true"

        # If commit → call our own Decision.Prepare
        if vote_commit:
            try:
                log_client("Vote", "Prepare", "Decision", NODE_ID)
                channel = grpc.insecure_channel(f"localhost:{DECISION_PORT}")
                stub = rpc.DecisionServiceStub(channel)

                prep_req = pb.PrepareRequestMsg(
                    txn=request.txn,
                    from_node=NODE_ID,
                    from_phase="Vote"
                )
                stub.Prepare(prep_req)

            except Exception as e:
                vote_commit = False
                reason = f"Local prepare failed: {e}"

        return pb.VoteResponse(vote_commit=vote_commit, reason=reason)


def serve_vote():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    rpc.add_VoteServiceServicer_to_server(VoteServiceServicer(), server)
    server.add_insecure_port(f"[::]:{VOTE_PORT}")
    server.start()
    print(f"[{NODE_ID}] Vote phase gRPC on :{VOTE_PORT}", flush=True)
    return server
