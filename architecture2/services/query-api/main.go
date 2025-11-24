package main
import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
	"google.golang.org/grpc"
	pb "github.com/Abdullah007noman/My-Distributed-System/proto"
)

type server struct{ pb.UnimplementedQueryAPIServer }

func (s *server) Range(ctx context.Context, req *pb.QueryRequest) (*pb.QueryReply, error) {
	points := []*pb.QueryPoint{}
	now := time.Now()
	for i := 0; i < 10; i++ {
		points = append(points, &pb.QueryPoint{
			TsUnixMs:    now.Add(time.Duration(i)*time.Second).UnixMilli(),
			Temperature: 20 + float64(i),
			Humidity:    50 + float64(i),
		})
	}
	return &pb.QueryReply{Points: points}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50052")
	if err != nil { log.Fatal(err) }
	s := grpc.NewServer()
	pb.RegisterQueryAPIServer(s, &server{})
	fmt.Println("QueryAPI gRPC server listening on :50052")
	log.Fatal(s.Serve(lis))
}
