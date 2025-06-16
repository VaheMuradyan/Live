package main

import (
	"github.com/VaheMuradyan/Live/db"
	live "github.com/VaheMuradyan/Live/proto"
	"google.golang.org/grpc"
	"log"
	"net"
)

func main() {
	db.Connect()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := NewServer()
	s := grpc.NewServer()
	live.RegisterCoefficientServiceServer(s, server)

	go func() {
		StartClient()
	}()

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
