# Distributed System – Two-Phase Commit (2PC) Fault Tolerance
This project extends the `architecture1` Python-based distributed system by adding
a full **Two-Phase Commit (2PC)** protocol to provide fault tolerance across **five replicated nodes**.

Each node runs:

- The existing service logic (e.g., `alerter.py`)
- A **Vote Phase** gRPC server
- A **Decision Phase** gRPC server

Node **service-node1** acts as the **Coordinator** for the 2PC protocol.


---

# 🚀 Running the 2PC System


From inside `architecture1/` run:  #please go inside the folder name 'architecture1'

```bash
docker compose -f docker-compose.2pc.yml up --build


#to force an abort, set one node (e.g., service-node3) to vote NO:
- edit docker-compose.2pc.yml
service-node3:
  environment:
    NODE_ID: "service-node3"
    ABORT_SELF: "true"    <--- to  vote abort

```bash
docker compose -f docker-compose.2pc.yml up --build





