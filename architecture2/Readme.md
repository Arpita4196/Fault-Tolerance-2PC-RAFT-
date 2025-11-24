

# Distributed System – Raft Fault Tolerance
This project extends the `architecture2` go-based microservices architecture to implement Raft.
Complete **Raft** fault tolerance is implemented on the systems and 5 nodes are created for fault tolerance.

# 🚀 Running Raft

From inside `architecture2/` run:  #please go inside the folder name 'architecture2'

```bash
git clone https://github.com/Arpita4196/Fault-Tolerance-2PC-RAFT-.git
cd Fault-Tolerance-2PC-RAFT-/architecture2  #please go inside the folder name 'architecture2'
docker compose -f docker-compose.raft.yml up -d --build
```
Run the below Test Cases:

## Test Case 1: Leader Election

### Steps:

1. **Start the cluster:**
   ```bash
   docker compose -f docker-compose.raft.yml up -d --build
   ```

2. **Wait 15-20 seconds** for all nodes to start and elect a leader.
   
   **Important**: Leader election can take up to 3 seconds (election timeout), and nodes need time to start up. Wait at least 15 seconds to ensure a leader is elected.


3. **Check logs for leader election:**
   ```bash
   # Check for RequestVote RPCs (election activity)
   docker compose -f docker-compose.raft.yml logs -f
   ```
   
   **Expected**: You should see:
   - Multiple "Node X sends RPC RequestVote to Node Y" messages
   - "Node X runs RPC RequestVote called by Node Y" messages
   
   **To identify the leader**, Check which node is sending AppendEntries:**
   ```bash
   # Find which node is actively sending AppendEntries
   for i in 1 2 3 4 5; do
     if docker logs node$i 2>&1 | tail -5 | grep -q "sends RPC AppendEntries"; then
       echo "Node$i is sending AppendEntries (the leader)"
       docker logs node$i 2>&1 | grep "sends RPC AppendEntries" | tail -3
     fi
   done
   ```

   **If no leader found, wait longer and check again:**

4. **Check for heartbeats:**
   ```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```
   
   **Expected**: Every ~1 second, you should see:
   - "Node <leader_id> sends RPC AppendEntries to Node <follower_id>" (4 times per heartbeat, one for each follower)

 5. **Monitor heartbeats in real-time:**
   ```bash
   # Watch for AppendEntries messages (should appear every 1 second)
   docker compose -f docker-compose.raft.yml logs -f | grep "sends RPC AppendEntries"
   ```
   
   **Expected**: Heartbeats every 1 second from the leader to all followers.


## Test Case 2: All the followers receiving Heartbeats

### Steps:

1. **Start the cluster:** Ignore if already running from previous test case. Can directly jump to step 4.
   ```bash
   docker compose -f docker-compose.raft.yml up -d --build
   ```

2. **Wait 15-20 seconds** for all nodes to start and elect a leader.
   
   **Important**: Leader election can take up to 3 seconds (election timeout), and nodes need time to start up. Wait at least 15 seconds to ensure a leader is elected.


3. **Check logs for leader election:**
   ```bash
   # Check for RequestVote RPCs (election activity)
   docker compose -f docker-compose.raft.yml logs -f
   ```
   
   **Expected**: You should see:
   - Multiple "Node X sends RPC RequestVote to Node Y" messages
   - "Node X runs RPC RequestVote called by Node Y" messages
   
   **To identify the leader**, Check which node is sending AppendEntries:**
   ```bash
   # Find which node is actively sending AppendEntries
   for i in 1 2 3 4 5; do
     if docker logs node$i 2>&1 | tail -5 | grep -q "sends RPC AppendEntries"; then
       echo "Node$i is sending AppendEntries (likely the leader)"
       docker logs node$i 2>&1 | grep "sends RPC AppendEntries" | tail -3
     fi
   done
   ```

   **If no leader found, wait longer and check again:**

4. **Check for heartbeats:**
   ```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```
   
   **Expected**: Every ~1 second, you should see:
   - "Node <leader_id> sends RPC AppendEntries to Node <follower_id>" (4 times per heartbeat, one for each follower)

5. **Monitor heartbeats in real-time:**
   ```bash
   docker compose -f docker-compose.raft.yml logs -f
   ```
   
   **Expected**: Heartbeats from all the nodes bith leader and followers.
   - e.g. Node node5 sends RPC AppendEntries to Node node2
		  Node node2 runs RPC AppendEntries called by Node node5

6. **Monitor heartbeats of specific follower:** Update the node if node1 is the leader to some other follower node.
   ```bash
   docker logs node1
   ```
   
   **Expected**: Heartbeats from all the nodes bith leader and followers.
   - e.g. Node node5 sends RPC AppendEntries to Node node2
		  Node node2 runs RPC AppendEntries called by Node node5


## Test Case 3: Client Sends Ingestion to a Follower (Forwarding) -- When Ingestion is sent to follower it should get forwarded to leader, and follower should get acknowledgement from leader.

### Steps:

1. **Ensure cluster is running** (from previous Test Case or restart if needed)

2. **Identify which node is the leader:**
   ```bash
   docker compose -f docker-compose.raft.yml logs | grep "sends RPC AppendEntries" | tail -5
   ```
   The node sending AppendEntries is the leader. Note its ID (e.g., "node1").

3. **Already I have created a file to connect to follower. Will run that file.** Make sure you are in atchitecture2 folder before running below code.

	**Important**: Before running, check which node is the leader:
	```bash
   docker compose -f docker-compose.raft.yml logs | grep "sends RPC AppendEntries" | tail -1
   ```

   - If node2 (port 50052) is the leader, edit `test_follower.go` and change `50052` to a different follower's port (50051, 50053, 50054, or 50055)
   
   ```bash
   cd client
   go build -o test_follower test_follower.go
   ./test_follower
   cd ..
   ```

 4. **Check logs for forwarding:**

 	- Change the node in below run if you have updated the code in before step to some other node.

   ```bash
   docker logs node2 | grep -E "forwarding|ingestion"
   ```
   
   **Expected**: You should see:
   - "Node node2 forwarding request to leader at node1:50051" (or similar)
   - The request should be processed successfully

5. **Verify the reading was processed:**
   ```bash
   LEADER=$(docker compose -f docker-compose.raft.yml logs | grep "sends RPC AppendEntries" | tail -1 | grep -o "Node [^ ]*" | awk '{print $2}')
   echo "Leader is: $LEADER"```
   
   - Check leader logs for AppendEntries being sent (this happens when leader processes the request), Before running replace node1 with leader node with the leader node. 
   ```bash
   docker logs node1 | grep "sends RPC AppendEntries" | tail -5
   ```
   
   **What you should see**: 
   - The leader sending AppendEntries RPCs to replicate the log entry
   - Example: `Node node1 sends RPC AppendEntries to Node node2`
   - This confirms the leader received and processed the forwarded request
   
   **Alternative**: Check all recent activity on the leader: Before running replace node1 with leader node with the leader node. 
   ```bash
   docker logs node1 | tail -20
   ```
## Test Case 5: Node failure/rejoins

**Objective**: Verify that when the node fails the leader stops sendin heartbeat to the node and when it is up and running it starts sending the heartbeat**

1. **Start fresh cluster:**
	First down all the clusters then rebuild it
	```bash
	docker compose -f docker-compose.raft.yml down
	```
   ```bash
   docker compose -f docker-compose.raft.yml up -d --build
   ```

2. **Wait 15-20 seconds** for all nodes to start and elect a leader.
   
   **Important**: Leader election can take up to 3 seconds (election timeout), and nodes need time to start up. Wait at least 15 seconds to ensure a leader is elected.


3. **Check logs for the nodes and when append entries start then leader is elected:**
   ```bash
   docker compose -f docker-compose.raft.yml logs -f
   ```

   Also check the heartbeats 
   ```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```
   **If no leader found, wait longer and check again:**
   - Note down the lead node

4. **Stop the node:** Update the node and run make sure it is nit leader node e.g. considering here node2 as the non leader node
   ```bash
   docker stop node2
   ```

5. **Verify logs of the leader and check that it is not sending heartbeats to stopped node**
	```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```

   Verify that stopped node is not receiving the heartbeats

6. **Start the node back up**
```bash
   docker start node2
   ```

7. **Verify logs of the leader and check that it is sending heartbeats to restarted node**
	```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```

   Verify that restrted node is receiving the heartbeats

## Test Case 5: Leader Failure + Re-election

**Objective**: Verify that when the leader fails, a new leader is elected and **only ONE leader exists at a time**

1. **Start the cluster:**
   First down all the clusters then rebuild it
	```bash
	docker compose -f docker-compose.raft.yml down
	```
   ```bash
   docker compose -f docker-compose.raft.yml up -d --build
   ```

2. **Wait 15-20 seconds** for all nodes to start and elect a leader.
   
   **Important**: Leader election can take up to 3 seconds (election timeout), and nodes need time to start up. Wait at least 15 seconds to ensure a leader is elected.


3. **Check logs for the nodes and when append entries start then leader is elected:**
   ```bash
   docker compose -f docker-compose.raft.yml logs -f
   ```

   Also check the heartbeats 
   ```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```
   **If no leader found, wait longer and check again:**
   - Note down the lead node

4. **Stop the leader:** Update the leader node and run e.g. considering here node4 as the leader
   ```bash
   docker stop node4
   ```

5. **Monitor remaining nodes for re-election:**
   # Wait for election timeout (3-5seconds)
   
   # Check for election activity (RequestVote RPCs)
   **Check logs for the nodes and when append entries start then leader is elected:**
   ```bash
   docker compose -f docker-compose.raft.yml logs -f
   ```

   Also check the heartbeats 
   ```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```
   **If no leader found, wait longer and check again:**
   
   **Expected Output**: Should see nodes sending RequestVote RPCs and starting elections.

6. **Verify new leader is elected:**
   ```bash
   docker compose -f docker-compose.raft.yml logs | grep -E "sends RPC AppendEntries"
   ```
   - If you are seeing append entries of the old leader wait and run again you should see the entries of the new leader.






