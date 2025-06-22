package client

import (
	"github.com/VaheMuradyan/Live/generator"
	live "github.com/VaheMuradyan/Live/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"time"
)

func StartClient() {
	time.Sleep(3 * time.Second)

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := live.NewCoefficientServiceClient(conn)

	generator := generator.NewCoefficientGenerator(client)

	if err = generator.StartAllSportsGeneration(); err != nil {
		log.Fatalf("Failed to start coefficient generation: %v", err)
	}

	select {}
}
