package main

import (
	"context"
	"fmt"
	"github.com/VaheMuradyan/Live/db"
	"github.com/VaheMuradyan/Live/models"
	live "github.com/VaheMuradyan/Live/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log"
	"time"
)

type CoefficientGenerator struct {
	client live.CoefficientServiceClient
}

func NewCoefficientGenerator() *CoefficientGenerator {
	return &CoefficientGenerator{}
}

func StartClient() {
	time.Sleep(3 * time.Second)

	generator := NewCoefficientGenerator()

	if err := generator.Connect(); err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	if err := generator.StartAllSportsGeneration(); err != nil {
		log.Fatalf("Failed to start coefficient generation: %v", err)
	}

	select {}
}

func (cg *CoefficientGenerator) Connect() error {
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %v", err)
	}
	cg.client = live.NewCoefficientServiceClient(conn)
	return nil
}

func (cg *CoefficientGenerator) StartAllSportsGeneration() error {
	var sports []models.Sport
	err := db.DB.Find(&sports).Error

	if err != nil {
		return fmt.Errorf("failed to get sports from database: %v", err)
	}

	intervals := []uint32{2, 3, 4}
	for i, sport := range sports {
		if err := cg.startSportWithInterval(sport.Name, intervals[i]); err != nil {
			fmt.Printf("Error starting %s: %v\n", sport.Name, err)
			continue
		}
	}

	return nil
}

func (cg *CoefficientGenerator) startSportWithInterval(sportName string, interval uint32) error {
	req := &live.SportRequest{
		Sport:                 sportName,
		UpdateIntervalSeconds: interval,
	}

	resp, err := cg.client.StartSportUpdates(context.Background(), req)
	if err != nil {
		return err
	}

	if resp.GetSuccess() {
		return nil
	}

	return fmt.Errorf(resp.GetMessage())
}
