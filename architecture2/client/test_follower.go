package main

import (
	"context"
	"fmt"
	"log"
	"time"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "github.com/Abdullah007noman/My-Distributed-System/proto"
)

func main() {
	// Connect to node2 (port 50052) - change this port if node2 is the leader
	// To find the leader, run: docker compose -f docker-compose.raft.yml logs | grep "sends RPC AppendEntries" | tail -1
	// If node2 is leader, change 50052 to 50051, 50053, 50054, or 50055 for a different follower
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	
	client := pb.NewIngestionClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := client.StreamReadings(ctx)
	if err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}
	
	reading := &pb.SensorReading{
		SensorId:    "sensor-test-2",
		TsUnixMs:    time.Now().UnixMilli(),
		Site:        "test-site",
		Temperature: 25.5,
		Humidity:    60.0,
		Seq:         1,
	}
	
	if err := stream.Send(reading); err != nil {
		log.Fatalf("Send error: %v", err)
	}
	fmt.Printf("✅ Sent reading to follower\n")
	
	ack, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Receive error: %v", err)
	}
	fmt.Printf("✅ Received ack: %d\n", ack.LastSeq)
}

