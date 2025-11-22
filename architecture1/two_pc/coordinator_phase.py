import os
import time
import grpc

import two_phase_pb2 as pb
import two_phase_pb2_grpc as rpc

NODE_ID = os.environ.get("NODE_ID", "service-node1")
VOTE_PORT = int(os.environ.get("VOTE_PORT", "50052"))
DECISION_PORT = int(os.environ.get("DECISION_PORT", "50053"))

CLUSTER = [
    "service-node1",
    "service-node2",
    "service-node3",
    "service-node4",
    "service-node5",
]


def log_client(from_phase, rpc_name, to_phase, to_node):
    print(
        f"Phase {from_phase} of Node {NODE_ID} sends RPC {rpc_name} "
        f"to Phase {to_phase} of Node {to_node}",
        flush=True
    )


def run_coordinator():
    time.sleep(3)

    txn = pb.Transaction(id="T101", payload="test-event")

    print(f"\n[{NODE_ID}] Starting 2PC for {txn.id}\n", flush=True)

    # ---------- PHASE 1: VOTING ----------
    votes = {}

    for node in CLUSTER:
        try:
            log_client("Coordinator", "VoteRequest", "Vote", node)

            channel = grpc.insecure_channel(f"{node}:{VOTE_PORT}")
            stub = rpc.VoteServiceStub(channel)

            req = pb.VoteRequestMsg(
                txn=txn,
                from_node=NODE_ID,
                from_phase="Coordinator"
            )

            resp = stub.VoteRequest(req)
            votes[node] = resp.vote_commit

        except Exception as e:
            print(f"[{NODE_ID}] ERROR contacting {node}: {e}", flush=True)
            votes[node] = False

    final_decision = "COMMIT" if all(votes.values()) else "ABORT"

    # ---------- PHASE 2: DECISION ----------
    for node in CLUSTER:

        channel = grpc.insecure_channel(f"{node}:{DECISION_PORT}")
        stub = rpc.DecisionServiceStub(channel)

        req = pb.DecisionRequestMsg(
            txn=txn,
            from_node=NODE_ID,
            from_phase="Coordinator"
        )

        if final_decision == "COMMIT":
            log_client("Coordinator", "GlobalCommit", "Decision", node)
            stub.GlobalCommit(req)
        else:
            log_client("Coordinator", "GlobalAbort", "Decision", node)
            stub.GlobalAbort(req)

    print(f"\n[{NODE_ID}] FINAL DECISION for {txn.id} = {final_decision}\n",
          flush=True)
