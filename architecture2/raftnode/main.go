// Package main implements a simplified Raft node integrated with the IoT
// Sensor Monitoring System.  Each node participates in leader election
// and log replication.  When running as the leader it accepts sensor
// readings and replicates them to followers.  The log entries are
// applied to a Redis stream once committed.

package raftnode

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "math/rand"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/redis/go-redis/v9"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    raftpb "github.com/Abdullah007noman/My-Distributed-System/raftpb"
)

// Role enumerates the possible states of a node.
type Role int

const (
    follower Role = iota
    candidate
    leader
)

// LogEntry corresponds to raftpb.LogEntry but we embed the struct
// directly to avoid confusion.
type LogEntry = raftpb.LogEntry

// SensorReading defines the structure of sensor telemetry that will
// be replicated through the Raft log.  It mirrors the fields from
// telemetry.proto but is kept simple here.
type SensorReading struct {
    SensorId    string  `json:"sensor_id"`
    TsUnixMs    int64   `json:"ts_unix_ms"`
    Site        string  `json:"site"`
    Temperature float64 `json:"temperature"`
    Humidity    float64 `json:"humidity"`
}

// RaftNode implements the raftpb.RaftServer interface and holds
// state required for leader election and log replication.
type RaftNode struct {
    raftpb.UnimplementedRaftServer
    Id         string
    peers      []string
    role       Role
    mu         sync.Mutex
    term       uint64
    votedFor   string
    log        []LogEntry
    commitIdx  uint64
    lastApplied uint64
    nextIndex  map[string]uint64
    matchIndex map[string]uint64
    leaderId   string
    electionReset chan struct{}
    stopHeartbeat chan struct{}  // Channel to signal heartbeat loop to stop
    redis      *redis.Client
}

// NewRaftNode constructs a new node.  It expects environment variables
// NODE_ID, PEERS (comma‑separated host:port list) and REDIS_ADDR to
// configure the node.  A random seed is set for election timeouts.
func NewRaftNode() *RaftNode {
    id := os.Getenv("NODE_ID")
    peersEnv := os.Getenv("PEERS")
    var peers []string
    if peersEnv != "" {
        peers = strings.Split(peersEnv, ",")
    }
    redisAddr := os.Getenv("REDIS_ADDR")
    if redisAddr == "" {
        redisAddr = "redis:6379"
    }
    rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
    rand.Seed(time.Now().UnixNano())
    return &RaftNode{
        Id:       id,
        peers:    peers,
        role:     follower,
        term:     0,
        votedFor: "",
        log:      []LogEntry{{Index: 0, Term: 0}}, // log index starts at 1; element 0 is dummy
        commitIdx: 0,
        lastApplied: 0,
        nextIndex: make(map[string]uint64),
        matchIndex: make(map[string]uint64),
        electionReset: make(chan struct{}, 1),
        stopHeartbeat: make(chan struct{}, 1),
        redis:     rdb,
    }
}

// RequestVote handles a RequestVote RPC from another node.  It
// implements the logic described in the Raft paper: granting one
// vote per term to the candidate whose log is at least as up to date.
func (rn *RaftNode) RequestVote(ctx context.Context, req *raftpb.RequestVoteRequest) (*raftpb.RequestVoteReply, error) {
    rn.mu.Lock()
    defer rn.mu.Unlock()
    // Print server‑side message
    log.Printf("Node %s runs RPC RequestVote called by Node %s", rn.Id, req.CandidateId)
    reply := &raftpb.RequestVoteReply{Term: rn.term, VoteGranted: false}
    // If the request’s term is less than our current term, reject
    if req.Term < rn.term {
        return reply, nil
    }
    // If the request's term is greater, update our term and convert to follower
    if req.Term > rn.term {
        log.Printf("Node %s: Received higher term %d, stepping down from %v to follower", rn.Id, req.Term, rn.role)
        rn.term = req.Term
        rn.votedFor = ""
        rn.role = follower
    }
    // Check if we have already voted in this term
    if rn.votedFor == "" || rn.votedFor == req.CandidateId {
        // Compare logs: candidate's log must be at least as up to date
        lastIdx := rn.lastLogIndex()
        lastTerm := rn.lastLogTerm()
        if req.LastLogTerm > lastTerm || (req.LastLogTerm == lastTerm && req.LastLogIndex >= lastIdx) {
            rn.votedFor = req.CandidateId
            reply.VoteGranted = true
            // Reset election timer since we heard from candidate
            rn.resetElectionTimer()
        }
    }
    reply.Term = rn.term
    return reply, nil
}

// AppendEntries handles AppendEntries RPC from the leader.  This
// function acts both as heartbeat and log replication.  It returns
// success if the follower’s log contains an entry matching
// prev_log_index and prev_log_term and then appends any new entries.
func (rn *RaftNode) AppendEntries(ctx context.Context, req *raftpb.AppendEntriesRequest) (*raftpb.AppendEntriesReply, error) {
    rn.mu.Lock()
    defer rn.mu.Unlock()
    // Print server‑side message
    log.Printf("Node %s runs RPC AppendEntries called by Node %s", rn.Id, req.LeaderId)
    resp := &raftpb.AppendEntriesReply{Term: rn.term, Success: false, MatchIndex: 0}
    
    // First, reject if term is lower (this is the standard Raft check)
    if req.Term < rn.term {
        return resp, nil
    }
    
    // If we receive a higher term, update our term and convert to follower
    if req.Term > rn.term {
        log.Printf("Node %s: Received higher term %d, stepping down from %v to follower", rn.Id, req.Term, rn.role)
        rn.term = req.Term
        rn.votedFor = ""
        rn.role = follower
        rn.leaderId = req.LeaderId
        // Signal heartbeat loop to stop if we were leader
        if rn.role == follower {
            select {
            case rn.stopHeartbeat <- struct{}{}:
            default:
            }
        }
    }
    
    // Reset election timer on heartbeat/append
    rn.resetElectionTimer()
    if rn.leaderId != req.LeaderId {
        log.Printf("Node %s: Updating leaderId from '%s' to '%s'", rn.Id, rn.leaderId, req.LeaderId)
    }
    rn.leaderId = req.LeaderId
    // Check if log contains entry at prev_log_index with term prev_log_term
    if req.PrevLogIndex > uint64(len(rn.log)-1) || rn.log[req.PrevLogIndex].Term != req.PrevLogTerm {
        // If follower is missing entries or terms mismatch, reject
        return resp, nil
    }
    // According to Q4: leader sends entire log, so we replace all entries after prevLogIndex
    // Truncate log to keep only entries up to and including prevLogIndex
    rn.log = rn.log[:req.PrevLogIndex+1]
    // Append all new entries from leader
    for _, entry := range req.Entries {
        rn.log = append(rn.log, *entry)
    }
    // Update commit index
    if req.LeaderCommit > rn.commitIdx {
        rn.commitIdx = min(req.LeaderCommit, uint64(len(rn.log)-1))
        rn.applyCommittedEntries()
    }
    resp.Term = rn.term
    resp.Success = true
    resp.MatchIndex = req.PrevLogIndex + uint64(len(req.Entries))
    return resp, nil
}

// GetLeader returns the current leader ID and term. This allows
// followers to forward client requests to the leader.
func (rn *RaftNode) GetLeader(ctx context.Context, req *raftpb.GetLeaderRequest) (*raftpb.GetLeaderReply, error) {
    rn.mu.Lock()
    defer rn.mu.Unlock()
    return &raftpb.GetLeaderReply{
        LeaderId: rn.leaderId,
        Term:     rn.term,
    }, nil
}

// IsLeader returns true if this node is currently the leader.
func (rn *RaftNode) IsLeader() bool {
    rn.mu.Lock()
    defer rn.mu.Unlock()
    return rn.role == leader
}

// GetLeaderAddress returns the full address (host:port) of the current leader.
// It constructs the address from the leader ID by extracting the port from PEERS.
func (rn *RaftNode) GetLeaderAddress() string {
    rn.mu.Lock()
    defer rn.mu.Unlock()
    log.Printf("Node %s: GetLeaderAddress() - leaderId='%s', role=%v, peers=%v", rn.Id, rn.leaderId, rn.role, rn.peers)
    if rn.leaderId == "" {
        log.Printf("Node %s: GetLeaderAddress() - leaderId is empty", rn.Id)
        return ""
    }
    // If we're the leader, return our own address
    if rn.role == leader {
        // Find our own address from NODE_PORT
        port := os.Getenv("NODE_PORT")
        if port == "" {
            port = "50051"
        }
        addr := fmt.Sprintf("%s:%s", rn.Id, port)
        log.Printf("Node %s: GetLeaderAddress() - I am leader, returning %s", rn.Id, addr)
        return addr
    }
    // Find the leader in peers list
    for _, peer := range rn.peers {
        if strings.HasPrefix(peer, rn.leaderId+":") {
            log.Printf("Node %s: GetLeaderAddress() - found leader in peers: %s", rn.Id, peer)
            return peer
        }
    }
    // If not found in peers, try to construct from NODE_PORT pattern
    // For node1->50051, node2->50052, etc.
    if strings.HasPrefix(rn.leaderId, "node") {
        nodeNum := rn.leaderId[len("node"):]
        port := 50050
        if num, err := strconv.Atoi(nodeNum); err == nil {
            port = 50050 + num
        }
        addr := fmt.Sprintf("%s:%d", rn.leaderId, port)
        log.Printf("Node %s: GetLeaderAddress() - constructed address from pattern: %s", rn.Id, addr)
        return addr
    }
    log.Printf("Node %s: GetLeaderAddress() - failed to construct address", rn.Id)
    return ""
}

// DiscoverLeader queries peers to find the current leader. This is used
// as a fallback when GetLeaderAddress() returns empty (e.g., before first heartbeat).
func (rn *RaftNode) DiscoverLeader() string {
    rn.mu.Lock()
    peers := make([]string, len(rn.peers))
    copy(peers, rn.peers)
    rn.mu.Unlock()
    
    log.Printf("Node %s: DiscoverLeader() querying %d peers: %v", rn.Id, len(peers), peers)
    
    // Query each peer to find the leader
    for _, peer := range peers {
        log.Printf("Node %s: Querying peer %s for leader", rn.Id, peer)
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        conn, err := grpc.DialContext(ctx, peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
        if err != nil {
            log.Printf("Node %s: Failed to dial peer %s: %v", rn.Id, peer, err)
            cancel()
            continue
        }
        
        client := raftpb.NewRaftClient(conn)
        reply, err := client.GetLeader(ctx, &raftpb.GetLeaderRequest{})
        conn.Close()
        cancel()
        
        if err != nil {
            log.Printf("Node %s: GetLeader RPC failed for peer %s: %v", rn.Id, peer, err)
            continue
        }
        
        if reply.LeaderId == "" {
            log.Printf("Node %s: Peer %s returned empty leaderId", rn.Id, peer)
            continue
        }
        
        log.Printf("Node %s: Peer %s reported leader: %s", rn.Id, peer, reply.LeaderId)
        
        // Found a leader, update our leaderId and return the address
        rn.mu.Lock()
        rn.leaderId = reply.LeaderId
        rn.mu.Unlock()
        
        // Construct address from leader ID
        for _, p := range peers {
            if strings.HasPrefix(p, reply.LeaderId+":") {
                log.Printf("Node %s: Found leader address: %s", rn.Id, p)
                return p
            }
        }
        // Fallback to pattern matching
        if strings.HasPrefix(reply.LeaderId, "node") {
            nodeNum := reply.LeaderId[len("node"):]
            port := 50050
            if num, err := strconv.Atoi(nodeNum); err == nil {
                port = 50050 + num
            }
            addr := fmt.Sprintf("%s:%d", reply.LeaderId, port)
            log.Printf("Node %s: Constructed leader address: %s", rn.Id, addr)
            return addr
        }
    }
    log.Printf("Node %s: DiscoverLeader() failed - no leader found in any peer", rn.Id)
    return ""
}

// submitReading receives a SensorReading from a client.  If this node
// is not the leader it returns an error so the caller can retry on
// the leader.  Otherwise it appends the reading to the log and
// replicates it to followers, committing once a majority have
// acknowledged.
// According to Q4: leader sends its entire log to all servers on next heartbeat.
func (rn *RaftNode) SubmitReading(reading SensorReading) error {
    rn.mu.Lock()
    if rn.role != leader {
        leaderId := rn.leaderId
        rn.mu.Unlock()
        return fmt.Errorf("not leader; please retry on %s", leaderId)
    }
    // Serialize reading to JSON
    data, _ := json.Marshal(reading)
    entry := LogEntry{
        Index: uint64(len(rn.log)),
        Term:  rn.term,
        Command: data,
    }
    rn.log = append(rn.log, entry)
    // Store commit index before unlocking
    commit := rn.commitIdx
    rn.mu.Unlock()
    
    // According to Q4: send entire log to all followers, not just new entry
    // Replicate to peers
    var wg sync.WaitGroup
    successCount := 1 // leader has appended entry to its own log
    responses := make(chan bool, len(rn.peers))
    for _, peer := range rn.peers {
        peer := peer // capture variable
        wg.Add(1)
        go func() {
            defer wg.Done()
            ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
            conn, err := grpc.DialContext(ctx, peer, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
            cancel()
            if err != nil {
                log.Printf("Node %s: Error dialing %s: %v", rn.Id, peer, err)
                responses <- false
                return
            }
            defer conn.Close()
            client := raftpb.NewRaftClient(conn)
            rn.mu.Lock()
            // Send entire log (Q4 requirement)
            // prevIndex=0, prevTerm=0 points to the dummy entry at index 0
            prevIndex := uint64(0)
            prevTerm := uint64(0)
            // Convert entire log to proto entries (excluding dummy entry at index 0)
            allEntries := make([]*raftpb.LogEntry, 0, len(rn.log)-1)
            for i := 1; i < len(rn.log); i++ {
                allEntries = append(allEntries, &rn.log[i])
            }
            leaderTerm := rn.term
            leaderCommit := commit
            rn.mu.Unlock()
            req := &raftpb.AppendEntriesRequest{
                Term:         leaderTerm,
                LeaderId:     rn.Id,
                PrevLogIndex: prevIndex,
                PrevLogTerm:  prevTerm,
                Entries:      allEntries, // Send entire log
                LeaderCommit: leaderCommit,
            }
            log.Printf("Node %s sends RPC AppendEntries to Node %s", rn.Id, extractNodeId(peer))
            resp, err := client.AppendEntries(context.Background(), req)
            if err != nil {
                log.Printf("AppendEntries error to %s: %v", peer, err)
                responses <- false
                return
            }
            if resp.Success {
                responses <- true
            } else {
                responses <- false
            }
        }()
    }
    // Wait for replies
    wg.Wait()
    close(responses)
    for ok := range responses {
        if ok {
            successCount++
        }
    }
    // Commit if a majority acknowledge
    if successCount*2 > len(rn.peers)+1 {
        rn.mu.Lock()
        rn.commitIdx = entry.Index
        rn.mu.Unlock()
        rn.applyCommittedEntries()
        return nil
    }
    return fmt.Errorf("failed to replicate to majority")
}

// ElectionLoop runs in a goroutine and initiates elections when the
// election timeout elapses.  When elected leader, it starts the
// heartbeat loop.  The loop runs indefinitely until the program exits.
func (rn *RaftNode) ElectionLoop() {
    for {
        timeout := time.Duration(1500+rand.Intn(1500)) * time.Millisecond
        timer := time.NewTimer(timeout)
        select {
        case <-rn.electionReset:
            timer.Stop()
            // proceed to next iteration to create a new timer
        case <-timer.C:
            rn.startElection()
        }
    }
}

// startElection transitions the node to candidate, increments term,
// votes for itself and sends RequestVote RPCs to peers.  If a
// majority of votes are received, it becomes leader and starts
// sending heartbeats.
func (rn *RaftNode) startElection() {
    rn.mu.Lock()
    // Don't start election if we're already a leader (shouldn't happen, but safety check)
    if rn.role == leader {
        log.Printf("Node %s: Already leader, skipping election", rn.Id)
        rn.mu.Unlock()
        return
    }
    rn.role = candidate
    rn.term++
    newTerm := rn.term
    rn.votedFor = rn.Id
    // Clear leaderId when starting new election (we're trying to become leader ourselves)
    rn.leaderId = ""
    lastIndex := rn.lastLogIndex()
    lastTerm := rn.lastLogTerm()
    rn.mu.Unlock()
    log.Printf("Node %s: Starting election in term %d", rn.Id, newTerm)
    votes := 1
    var wg sync.WaitGroup
    votesCh := make(chan bool, len(rn.peers))
    for _, peer := range rn.peers {
        peer := peer
        wg.Add(1)
        go func() {
            defer wg.Done()
            ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
            conn, err := grpc.DialContext(ctx, peer, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
            cancel()
            if err != nil {
                // This is expected if the peer is down, busy, or unreachable
                // Don't log as error - just count as no vote
                log.Printf("Node %s: Cannot connect to %s (timeout/unreachable) - counting as no vote", rn.Id, extractNodeId(peer))
                votesCh <- false
                return
            }
            defer conn.Close()
            client := raftpb.NewRaftClient(conn)
            req := &raftpb.RequestVoteRequest{
                Term:         rn.term,
                CandidateId:  rn.Id,
                LastLogIndex: lastIndex,
                LastLogTerm:  lastTerm,
            }
            log.Printf("Node %s sends RPC RequestVote to Node %s", rn.Id, extractNodeId(peer))
            resp, err := client.RequestVote(context.Background(), req)
            if err != nil {
                log.Printf("Node %s: RequestVote RPC error to %s: %v", rn.Id, extractNodeId(peer), err)
                votesCh <- false
                return
            }
            if resp.VoteGranted {
                votesCh <- true
            } else {
                votesCh <- false
            }
            // Update term if higher term observed
            if resp.Term > rn.term {
                rn.mu.Lock()
                rn.term = resp.Term
                rn.role = follower
                rn.votedFor = ""
                rn.mu.Unlock()
            }
        }()
    }
    wg.Wait()
    close(votesCh)
    // Count votes
    for granted := range votesCh {
        if granted {
            votes++
        }
    }
    log.Printf("Node %s: Election vote count: %d votes received (including self), need %d for majority of %d nodes", rn.Id, votes, (len(rn.peers)+1)/2+1, len(rn.peers)+1)
    
    // If majority votes, become leader
    rn.mu.Lock()
    // Check if we're still a candidate and term hasn't changed
    if rn.role != candidate {
        rn.mu.Unlock()
        return
    }
    if votes*2 > len(rn.peers)+1 {
        rn.role = leader
        rn.leaderId = rn.Id
        // Initialize nextIndex for each peer
        for _, p := range rn.peers {
            rn.nextIndex[p] = rn.lastLogIndex() + 1
            rn.matchIndex[p] = 0
        }
        finalTerm := rn.term
        rn.mu.Unlock()
        log.Printf("Node %s becomes leader in term %d", rn.Id, finalTerm)
        go rn.heartbeatLoop()
    } else {
        rn.mu.Unlock()
        log.Printf("Node %s: Election failed - only got %d votes, need majority of %d", rn.Id, votes, len(rn.peers)+1)
    }
}

// heartbeatLoop runs on the leader and periodically sends AppendEntries
// to all followers to maintain authority. According to Q4, it sends
// the entire log on each heartbeat. It exits if the node ceases to be leader.
func (rn *RaftNode) heartbeatLoop() {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-rn.stopHeartbeat:
            log.Printf("Node %s: heartbeatLoop stopping - received stop signal", rn.Id)
            return
        case <-ticker.C:
            rn.mu.Lock()
            // Double-check we're still leader before sending heartbeats
            if rn.role != leader {
                rn.mu.Unlock()
                log.Printf("Node %s: heartbeatLoop exiting - no longer leader", rn.Id)
                return
            }
            term := rn.term
            leaderId := rn.Id
            // According to Q4: send entire log on heartbeat
            // prevIndex=0, prevTerm=0 points to the dummy entry at index 0
            prevIndex := uint64(0)
            prevTerm := uint64(0)
            // Convert entire log to proto entries (excluding dummy entry at index 0)
            allEntries := make([]*raftpb.LogEntry, 0, len(rn.log)-1)
            for i := 1; i < len(rn.log); i++ {
                allEntries = append(allEntries, &rn.log[i])
            }
            commit := rn.commitIdx
            rn.mu.Unlock()
            // Check again after unlocking (in case we stepped down)
            rn.mu.Lock()
            if rn.role != leader {
                rn.mu.Unlock()
                log.Printf("Node %s: heartbeatLoop exiting - stepped down during heartbeat prep", rn.Id)
                return
            }
            rn.mu.Unlock()
            for _, peer := range rn.peers {
            peer := peer
            go func() {
                ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
                conn, err := grpc.DialContext(ctx, peer, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
                cancel()
                if err != nil {
                    log.Printf("Node %s: Dial error to %s: %v", leaderId, peer, err)
                    return
                }
                defer conn.Close()
                client := raftpb.NewRaftClient(conn)
                req := &raftpb.AppendEntriesRequest{
                    Term:         term,
                    LeaderId:     leaderId,
                    PrevLogIndex: prevIndex,
                    PrevLogTerm:  prevTerm,
                    Entries:      allEntries, // Send entire log (Q4 requirement)
                    LeaderCommit: commit,
                }
                log.Printf("Node %s sends RPC AppendEntries to Node %s", leaderId, extractNodeId(peer))
                _, _ = client.AppendEntries(context.Background(), req)
            }()
            }
        }
    }
}

// applyCommittedEntries applies entries up to commitIdx to the Redis
// stream.  It also updates lastApplied to reflect the last applied
// log index.  Only committed entries are applied.
func (rn *RaftNode) applyCommittedEntries() {
    for {
        rn.mu.Lock()
        if rn.lastApplied >= rn.commitIdx {
            rn.mu.Unlock()
            return
        }
        idx := rn.lastApplied + 1
        entry := rn.log[idx]
        rn.mu.Unlock()
        // Deserialize command to SensorReading
        var reading SensorReading
        if err := json.Unmarshal(entry.Command, &reading); err == nil {
            // Push to Redis stream 'readings'
            ctx := context.Background()
            _, err = rn.redis.XAdd(ctx, &redis.XAddArgs{
                Stream: "readings",
                Values: map[string]interface{}{
                    "sensor_id":   reading.SensorId,
                    "site":        reading.Site,
                    "ts_unix_ms":  reading.TsUnixMs,
                    "temperature": strconv.FormatFloat(reading.Temperature, 'f', -1, 64),
                    "humidity":    strconv.FormatFloat(reading.Humidity, 'f', -1, 64),
                },
            }).Result()
            if err != nil {
                log.Printf("Redis XAdd error: %v", err)
            }
        }
        rn.mu.Lock()
        rn.lastApplied = idx
        rn.mu.Unlock()
    }
}

// lastLogIndex returns the index of the last entry in the log.
func (rn *RaftNode) lastLogIndex() uint64 {
    return uint64(len(rn.log) - 1)
}

// lastLogTerm returns the term of the last entry in the log.
func (rn *RaftNode) lastLogTerm() uint64 {
    return rn.log[len(rn.log)-1].Term
}

// resetElectionTimer signals the election loop to reset its timer.
func (rn *RaftNode) resetElectionTimer() {
    select {
    case rn.electionReset <- struct{}{}:
    default:
    }
}

// Helper function to extract node ID from peer address (e.g., "node1:50051" -> "node1")
func extractNodeId(peer string) string {
    parts := strings.Split(peer, ":")
    if len(parts) > 0 {
        return parts[0]
    }
    return peer
}

// Helper function to compute minimum of two uint64 values.
func min(a, b uint64) uint64 {
    if a < b {
        return a
    }
    return b
}
