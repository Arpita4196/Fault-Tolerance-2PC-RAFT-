import os
import time
import threading

from vote_phase import serve_vote
from decision_phase import serve_decision
from coordinator_phase import run_coordinator

NODE_ID = os.environ.get("NODE_ID", "service-node1")
IS_COORDINATOR = os.environ.get("COORDINATOR", "false").lower() == "true"


def main():
    vote_server = serve_vote()
    decision_server = serve_decision()

    if IS_COORDINATOR:
        threading.Thread(target=run_coordinator, daemon=True).start()

    while True:
        time.sleep(3600)


if __name__ == "__main__":
    main()
