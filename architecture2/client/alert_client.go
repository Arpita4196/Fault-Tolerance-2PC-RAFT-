package main

import (
	"context"
	"log"

	"google.golang.org/grpc"
	pb "github.com/Abdullah007noman/My-Distributed-System/proto"
)

func main() {
	conn, err := grpc.Dial("localhost:50054", grpc.WithInsecure())
	if err != nil { log.Fatal(err) }
	defer conn.Close()
	c := pb.NewAlerterClient(conn)
	str, err := c.SubscribeAlerts(context.Background(), &pb.AlertRequest{})
	if err != nil { log.Fatal(err) }
	log.Println("Subscribed to alerts...")
	for {
		a, err := str.Recv()
		if err != nil { log.Fatal(err) }
		log.Printf("ALERT: %s/%s %s value=%.2f thr=%.2f", a.SensorId, a.Site, a.Reason, a.Value, a.Threshold)
	}
}
