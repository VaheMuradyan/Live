package generator

import (
	"context"
	"fmt"
	"github.com/VaheMuradyan/Live/db"
	"github.com/VaheMuradyan/Live/db/models"
	live "github.com/VaheMuradyan/Live/proto"
)

type CoefficientGenerator struct {
	client live.CoefficientServiceClient
}

func NewCoefficientGenerator(client live.CoefficientServiceClient) *CoefficientGenerator {
	return &CoefficientGenerator{
		client: client,
	}
}

func (cg *CoefficientGenerator) StartAllSportsGeneration() error {
	var sports []models.Sport
	err := db.DB.Find(&sports).Error

	if err != nil {
		return fmt.Errorf("failed to get sports from database: %v", err)
	}

	intervals := []uint32{3, 3, 3}
	for i, sport := range sports {
		if err := cg.startSportWithInterval(sport.Name, intervals[i]); err != nil {
			fmt.Printf("Error starting %s: %v\n", sport.Name, err)
			break
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
