package main

import (
	"context"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"

	pb "github.com/Abdullah007noman/My-Distributed-System/proto"
)

type metricWin struct {
	mu   sync.Mutex
	data map[string][]*pb.SensorReading // key: sensor|site
}

func newWin() *metricWin { return &metricWin{data: map[string][]*pb.SensorReading{}} }
func key(s, site string) string { return s + "|" + site }

func (w *metricWin) add(r *pb.SensorReading) *pb.Metric {
	w.mu.Lock()
	defer w.mu.Unlock()
	k := key(r.SensorId, r.Site)
	w.data[k] = append(w.data[k], r)
	// evict older than 10s
	cut := time.Now().Add(-10 * time.Second).UnixMilli()
	items := w.data[k]
	j := 0
	for _, it := range items {
		if it.TsUnixMs >= cut {
			items[j] = it
			j++
		}
	}
	items = items[:j]
	w.data[k] = items
	if len(items) == 0 {
		return nil
	}
	var st, sh float64
	for _, it := range items {
		st += it.Temperature
		sh += it.Humidity
	}
	return &pb.Metric{
		SensorId:    r.SensorId,
		Site:        r.Site,
		WindowMs:    10_000,
		AvgTemp:     st / float64(len(items)),
		AvgHumidity: sh / float64(len(items)),
		TsUnixMs:    time.Now().UnixMilli(),
	}
}

type aggServer struct {
	pb.UnimplementedAggregatorServer
	subsMu sync.Mutex
	subs   map[int]chan *pb.Metric
	nextID int
}

func newAggServer() *aggServer { return &aggServer{subs: map[int]chan *pb.Metric{}} }

func (s *aggServer) broadcast(m *pb.Metric) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subs {
		select { case ch <- m: default: }
	}
}

func (s *aggServer) SubscribeMetrics(req *pb.MetricsRequest, stream pb.Aggregator_SubscribeMetricsServer) error {
	s.subsMu.Lock()
	id := s.nextID
	s.nextID++
	ch := make(chan *pb.Metric, 256)
	s.subs[id] = ch
	s.subsMu.Unlock()

	defer func() {
		s.subsMu.Lock()
		delete(s.subs, id)
		s.subsMu.Unlock()
		close(ch)
	}()

	for m := range ch {
		if (req.SensorId == "" || req.SensorId == m.SensorId) &&
			(req.Site == "" || req.Site == m.Site) {
			if err := stream.Send(m); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "redis:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx := context.Background()
	_, _ = rdb.XGroupCreateMkStream(ctx, "readings", "agggrp", "$").Result()

	// gRPC
	lis, err := net.Listen("tcp", ":50053")
	if err != nil {
		log.Fatal(err)
	}
	gs := grpc.NewServer()
	srv := newAggServer()
	pb.RegisterAggregatorServer(gs, srv)
	win := newWin()

	// consume loop
	go func() {
		for {
			res, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    "agggrp",
				Consumer: "agg-1",
				Streams:  []string{"readings", ">"},
				Count:    200,
				Block:    5000 * time.Millisecond,
			}).Result()
			if err != nil && err != redis.Nil {
				log.Printf("XReadGroup error: %v", err)
				continue
			}
			for _, s := range res {
				for _, m := range s.Messages {
					r := &pb.SensorReading{}
					if v, ok := m.Values["sensor_id"].(string); ok { r.SensorId = v }
					if v, ok := m.Values["site"].(string); ok { r.Site = v }
					// For demo simplicity, set ts to now (type coercion varies by Redis drivers)
					r.TsUnixMs = time.Now().UnixMilli()
					if v, ok := m.Values["temperature"].(string); ok { _ = v }
					if v, ok := m.Values["humidity"].(string); ok { _ = v }

					if met := win.add(r); met != nil {
						srv.broadcast(met)
					}
					rdb.XAck(ctx, "readings", "agggrp", m.ID)
				}
			}
		}
	}()

	log.Println("Aggregator gRPC listening on :50053")
	if err := gs.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
