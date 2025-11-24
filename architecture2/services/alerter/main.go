package main

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	pb "github.com/Abdullah007noman/My-Distributed-System/proto"
)

type alertServer struct {
	pb.UnimplementedAlerterServer
	subsMu sync.Mutex
	subs   map[int]chan *pb.Alert
	nextID int
}

func newAlertServer() *alertServer { return &alertServer{subs: map[int]chan *pb.Alert{}} }

func (s *alertServer) broadcast(a *pb.Alert) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subs {
		select { case ch <- a: default: }
	}
}

func (s *alertServer) SubscribeAlerts(req *pb.AlertRequest, stream pb.Alerter_SubscribeAlertsServer) error {
	s.subsMu.Lock()
	id := s.nextID
	s.nextID++
	ch := make(chan *pb.Alert, 256)
	s.subs[id] = ch
	s.subsMu.Unlock()

	defer func() {
		s.subsMu.Lock()
		delete(s.subs, id)
		s.subsMu.Unlock()
		close(ch)
	}()

	for a := range ch {
		if (req.SensorId == "" || req.SensorId == a.SensorId) &&
			(req.Site == "" || req.Site == a.Site) {
			if err := stream.Send(a); err != nil {
				return err
			}
		}
	}
	return nil
}

func getenvFloat(name string, def float64) float64 {
	if v := os.Getenv(name); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil { return f }
	}
	return def
}

func main() {
	thrTemp := getenvFloat("THRESHOLD_TEMP", 28.0)
	thrHum  := getenvFloat("THRESHOLD_HUM",  70.0)

	// gRPC server (alerts out)
	lis, err := net.Listen("tcp", ":50054")
	if err != nil { log.Fatal(err) }
	gs := grpc.NewServer()
	s := newAlertServer()
	pb.RegisterAlerterServer(gs, s)

	// gRPC client (metrics in)
	go func() {
		for {
			conn, err := grpc.Dial("aggregator:50053", grpc.WithInsecure())
			if err != nil { log.Printf("dial aggregator: %v", err); time.Sleep(2*time.Second); continue }
			c := pb.NewAggregatorClient(conn)
			str, err := c.SubscribeMetrics(context.Background(), &pb.MetricsRequest{})
			if err != nil { log.Printf("subscribe: %v", err); conn.Close(); time.Sleep(2*time.Second); continue }
			log.Println("Alerter: subscribed to metrics")
			for {
				m, err := str.Recv()
				if err != nil { log.Printf("metrics recv: %v", err); conn.Close(); break }
				if m.AvgTemp > thrTemp {
					s.broadcast(&pb.Alert{
						SensorId: m.SensorId, Site: m.Site,
						Reason: "TEMP_HIGH", Value: m.AvgTemp, Threshold: thrTemp, TsUnixMs: time.Now().UnixMilli(),
					})
				}
				if m.AvgHumidity > thrHum {
					s.broadcast(&pb.Alert{
						SensorId: m.SensorId, Site: m.Site,
						Reason: "HUMID_HIGH", Value: m.AvgHumidity, Threshold: thrHum, TsUnixMs: time.Now().UnixMilli(),
					})
				}
			}
			time.Sleep(2*time.Second)
		}
	}()

	log.Println("Alerter gRPC listening on :50054")
	if err := gs.Serve(lis); err != nil { log.Fatal(err) }
}
