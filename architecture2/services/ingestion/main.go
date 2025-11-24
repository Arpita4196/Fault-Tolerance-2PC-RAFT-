package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/Abdullah007noman/My-Distributed-System/proto"
	raftpb "github.com/Abdullah007noman/My-Distributed-System/raftpb"
	raftnode "github.com/Abdullah007noman/My-Distributed-System/raftnode"
)

// server is the ingestion gRPC service, but now backed by Raft.
type server struct {
	pb.UnimplementedIngestionServer
	node *raftnode.RaftNode
}

func newServer(node *raftnode.RaftNode) *server {
	return &server{node: node}
}

// StreamReadings receives a stream of readings from sensors.
// Instead of writing directly to Redis, it goes through Raft.
// According to Q4: if this node is not the leader, forward the request to the leader.
func (s *server) StreamReadings(stream pb.Ingestion_StreamReadingsServer) error {
	// Check if we're the leader first
	isLeader := s.node.IsLeader()
	
	// If not leader, forward to leader (Q4 requirement)
	if !isLeader {
		log.Printf("Node %s: Not leader, getting leader address...", s.node.Id)
		leaderAddr := s.node.GetLeaderAddress()
		log.Printf("Node %s: GetLeaderAddress() returned: '%s'", s.node.Id, leaderAddr)
		if leaderAddr == "" {
			// Try to discover leader by querying peers
			log.Printf("Node %s: No leader known, attempting to discover leader from peers", s.node.Id)
			leaderAddr = s.node.DiscoverLeader()
			log.Printf("Node %s: DiscoverLeader() returned: '%s'", s.node.Id, leaderAddr)
			if leaderAddr == "" {
				log.Printf("Node %s: ERROR - No leader found after discovery attempt", s.node.Id)
				return fmt.Errorf("no leader known; cannot forward request")
			}
			log.Printf("Node %s: Discovered leader at %s", s.node.Id, leaderAddr)
		}
		log.Printf("Node %s forwarding request to leader at %s", s.node.Id, leaderAddr)
		
		// Forward to leader
		conn, err := grpc.Dial(leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Node %s: Failed to dial leader: %v", s.node.Id, err)
			return fmt.Errorf("failed to connect to leader %s: %v", leaderAddr, err)
		}
		defer conn.Close()
		log.Printf("Node %s: Connected to leader, creating forward stream", s.node.Id)
		
		leaderClient := pb.NewIngestionClient(conn)
		forwardStream, err := leaderClient.StreamReadings(stream.Context())
		if err != nil {
			log.Printf("Node %s: Failed to create forward stream: %v", s.node.Id, err)
			return fmt.Errorf("failed to create forward stream: %v", err)
		}
		log.Printf("Node %s: Forward stream created, waiting for client messages", s.node.Id)
		
		// Forward messages from client to leader
		for {
			log.Printf("Node %s: Waiting to receive message from client...", s.node.Id)
			msg, err := stream.Recv()
			if err == io.EOF {
				// Close forward stream and get ack
				log.Printf("Node %s: Client sent EOF, forwarding to leader", s.node.Id)
				ack, err := forwardStream.CloseAndRecv()
				if err != nil {
					log.Printf("Node %s: Error receiving ack from leader: %v", s.node.Id, err)
					return fmt.Errorf("failed to receive ack from leader: %v", err)
				}
				log.Printf("Node %s: Received ack from leader (LastSeq: %d), forwarding to client", s.node.Id, ack.LastSeq)
				return stream.SendAndClose(&pb.IngestAck{LastSeq: ack.LastSeq})
			}
			if err != nil {
				log.Printf("Node %s: Error receiving from client: %v", s.node.Id, err)
				return err
			}
			
			log.Printf("Node %s: Forwarding reading to leader", s.node.Id)
			if err := forwardStream.Send(msg); err != nil {
				log.Printf("Node %s: Error forwarding reading: %v", s.node.Id, err)
				return fmt.Errorf("failed to forward reading: %v", err)
			}
		}
	}
	
	// We're the leader, process locally
	var last uint64
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			log.Printf("Node %s (leader): Client sent EOF, sending ack with LastSeq: %d", s.node.Id, last)
			return stream.SendAndClose(&pb.IngestAck{LastSeq: last})
		}
		if err != nil {
			log.Printf("Node %s (leader): Error receiving from client: %v", s.node.Id, err)
			return err
		}

		last = msg.Seq
		log.Printf("Node %s (leader): Received reading (Seq: %d), submitting through Raft", s.node.Id, msg.Seq)

		// Submit through Raft
		err = s.node.SubmitReading(raftnode.SensorReading{
			SensorId:    msg.SensorId,
			Site:        msg.Site,
			TsUnixMs:    msg.TsUnixMs,
			Temperature: msg.Temperature,
			Humidity:    msg.Humidity,
		})
		
		if err != nil {
			log.Printf("Node %s (leader): ingestion error: %v", s.node.Id, err)
			return err
		}
		log.Printf("Node %s (leader): Successfully submitted reading (Seq: %d)", s.node.Id, msg.Seq)
	}
}

func main() {
	// Read NODE_PORT (must be different for each Raft node)
	port := os.Getenv("NODE_PORT")
	if port == "" {
		port = "50051"
	}

	// Sanity: PEERS should not include self
	if peers := os.Getenv("PEERS"); peers != "" {
		self := os.Getenv("NODE_ID")
		for _, p := range strings.Split(peers, ",") {
			if strings.Contains(p, self+":") {
				log.Printf("WARNING: PEERS contains self (%s). Remove it from compose.", p)
			}
		}
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", port, err)
	}

	// Create raft node
	node := raftnode.NewRaftNode()

	grpcServer := grpc.NewServer()

	// Register ingestion service (now Raft-backed)
	pb.RegisterIngestionServer(grpcServer, newServer(node))

	// Register Raft peer RPCs
	raftpb.RegisterRaftServer(grpcServer, node)

	// Start Raft election timer
	go node.ElectionLoop()

	log.Printf("Node %s starting Ingestion+Raft gRPC on :%s", node.Id, port)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve gRPC: %v", err)
	}
}
