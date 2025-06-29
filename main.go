package main

import (
	"github.com/VaheMuradyan/Live/client"
	"github.com/VaheMuradyan/Live/db"
	live "github.com/VaheMuradyan/Live/proto"
	mainServer "github.com/VaheMuradyan/Live/server"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
)

func main() {
	db.Connect()

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	lis, err := net.Listen("tcp", os.Getenv("GRPC_SERVER"))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	server := mainServer.NewServer()
	s := grpc.NewServer()
	live.RegisterCoefficientServiceServer(s, server)

	go client.StartClient()

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
