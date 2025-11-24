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
    // Connect to the QueryAPI service
    conn, err := grpc.Dial("localhost:50052", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Failed to connect to QueryAPI: %v", err)
    }
    defer conn.Close()

    client := pb.NewQueryAPIClient(conn)

    // Define query range (last 10 minutes)
    end := time.Now().UnixMilli()
    start := end - 10*60*1000 // 10 minutes ago

    req := &pb.QueryRequest{
        SensorId: "sensor-01",
        StartMs:  start,
        EndMs:    end,
    }

    resp, err := client.Range(context.Background(), req)
    if err != nil {
        log.Fatalf("Query failed: %v", err)
    }

    if len(resp.Points) == 0 {
        fmt.Println("⚠️  No data found in the given range.")
        return
    }

    fmt.Println("✅ Query Results:")
    for _, p := range resp.Points {
        ts := time.UnixMilli(p.TsUnixMs).Format("15:04:05")
        fmt.Printf("⏱ %s | 🌡 %.2f°C | 💧 %.2f%%\n", ts, p.Temperature, p.Humidity)
    }
}
