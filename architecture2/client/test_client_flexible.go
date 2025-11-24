package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	pb "github.com/Abdullah007noman/My-Distributed-System/proto"
)

func main() {
	// Parse command line arguments
	port := flag.String("port", "50051", "Port to connect to (default: 50051 for node1)")
	count := flag.Int("count", 5, "Number of readings to send (default: 5)")
	flag.Parse()

	addr := fmt.Sprintf("localhost:%s", *port)
	fmt.Printf("Connecting to %s...\n", addr)

	// Connect to the Ingestion service
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewIngestionClient(conn)

	// Start a stream
	stream, err := client.StreamReadings(context.Background())
	if err != nil {
		log.Fatalf("Failed to start stream: %v", err)
	}

	// Send sensor readings
	for i := 0; i < *count; i++ {
		reading := &pb.SensorReading{
			SensorId:    fmt.Sprintf("sensor-%02d", i+1),
			TsUnixMs:    time.Now().UnixMilli(),
			Site:        "test-lab",
			Temperature: 25.0 + float64(i),
			Humidity:    50.0 + float64(i),
			Seq:         uint64(i + 1),
		}

		if err := stream.Send(reading); err != nil {
			log.Fatalf("Send error: %v", err)
		}
		fmt.Printf("✅ Sent reading %d: sensor=%s, temp=%.1f, humidity=%.1f\n",
			i+1, reading.SensorId, reading.Temperature, reading.Humidity)
		time.Sleep(500 * time.Millisecond)
	}

	ack, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("Receive error: %v", err)
	}
	fmt.Printf("\n✅ Stream closed. Last seq acknowledged: %d\n", ack.LastSeq)
}

