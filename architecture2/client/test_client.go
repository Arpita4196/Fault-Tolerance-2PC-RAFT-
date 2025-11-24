
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "google.golang.org/grpc"
    pb "github.com/Abdullah007noman/My-Distributed-System/proto"

)

func main() {
    // Connect to the Ingestion service
    conn, err := grpc.Dial("localhost:50053", grpc.WithInsecure())
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

    // Send 5 fake sensor readings
    for i := 0; i < 2; i++ {
        reading := &pb.SensorReading{
            SensorId:    "sensor-01",
            TsUnixMs:    time.Now().UnixMilli(),
            Site:        "lab-1",
            Temperature: 25.0 + float64(i),
            Humidity:    50.0 + float64(i),
            Seq:         uint64(i + 1),
        }

        if err := stream.Send(reading); err != nil {
            log.Fatalf("Send error: %v", err)
        }
        fmt.Printf("✅ Sent reading %d\n", i+1)
        time.Sleep(500 * time.Millisecond)
    }

    ack, err := stream.CloseAndRecv()
    if err != nil {
        log.Fatalf("Receive error: %v", err)
    }
    fmt.Printf("✅ Stream closed. Last seq acknowledged: %d\n", ack.LastSeq)
}
