package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/VaheMuradyan/Live/db"
	"github.com/VaheMuradyan/Live/models"
	"github.com/VaheMuradyan/Live/proto"
	"github.com/go-resty/resty/v2"
)

const (
	apiKey = "0957bfe1-5aa9-40c0-991f-d15150f91594"
	apiUrl = "http://localhost:8000/api"
)

type Server struct {
	live.UnimplementedCoefficientServiceServer
	sportRoutines map[string]chan bool
	mu            sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		sportRoutines: make(map[string]chan bool),
	}
}

func (s *Server) StartSportUpdates(ctx context.Context, req *live.SportRequest) (*live.SportResponse, error) {
	sport := req.Sport
	interval := req.UpdateIntervalSeconds

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sportRoutines[sport]; exists {
		return &live.SportResponse{
			Success: false,
			Message: fmt.Sprintf("Sport %s updates already running", sport),
			Sport:   sport,
		}, nil
	}

	stopChan := make(chan bool)
	s.sportRoutines[sport] = stopChan

	go func(ticker *time.Ticker) {
		defer ticker.Stop()
		for {
			select {
			case <-stopChan:
				fmt.Println("Goroutine stopped")
				return
			case <-ticker.C:
				if err := s.generateCoefficientUpdate(sport); err != nil {
					log.Printf("Error updating coefficient for %s: %v", sport, err)
				}
			}
		}
	}(time.NewTicker(time.Duration(interval) * time.Second))

	return &live.SportResponse{
		Success: true,
		Message: fmt.Sprintf("Started %s coefficient updates", sport),
		Sport:   sport,
	}, nil
}

func (s *Server) runSportUpdates(sport string, interval time.Duration, stopChan chan bool) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopChan:
			fmt.Println("Goroutine stopped")
			return
		case <-ticker.C:
			if err := s.generateCoefficientUpdate(sport); err != nil {
				log.Printf("Error updating coefficient for %s: %v", sport, err)
			}
		}
	}
}

func (s *Server) generateCoefficientUpdate(sport string) error {
	prices, err := s.getPricesBySport(sport)
	if err != nil {
		return fmt.Errorf("error getting prices for %s: %v", sport, err)
	}

	randomPrice := prices[rand.Intn(len(prices))]
	return s.updateCoefficient(&randomPrice)
}

func (s *Server) updateCoefficient(price *models.Price) error {
	oldCoeff := price.CurrentCoefficient

	changePercent := (rand.Float64() - 0.5) * 0.4
	newCoeff := oldCoeff * (1 + changePercent)

	if newCoeff < 1.01 {
		newCoeff = 1.01
	}
	if newCoeff > 50.0 {
		newCoeff = 50.0
	}

	price.PreviousCoefficient = oldCoeff
	price.CurrentCoefficient = newCoeff
	price.LastUpdated = time.Now()

	if err := db.DB.Save(price).Error; err != nil {
		return fmt.Errorf("failed to update price in database: %v", err)
	}

	history := models.CoefficientHistory{
		MarketID: price.MarketID,
		OldValue: oldCoeff,
		NewValue: newCoeff,
	}
	db.DB.Create(&history)

	sport := price.Market.MarketCollection.Event.Competition.Country.Sport.Name

	data := map[string]interface{}{
		"sport":           sport,
		"country":         price.Market.MarketCollection.Event.Competition.Country.Name,
		"competition":     price.Market.MarketCollection.Event.Competition.Name,
		"event":           price.Market.MarketCollection.Event.Name,
		"market":          price.Market.Name,
		"price":           price.Name,
		"old_coefficient": float32(oldCoeff),
		"new_coefficient": float32(newCoeff),
		"timestamp":       time.Now().Format(time.RFC3339),
		"change":          float32(newCoeff) - float32(oldCoeff),
		"channel":         sport,
	}

	payload := map[string]interface{}{
		"method": "publish",
		"params": map[string]interface{}{
			"channel": strings.ToLower(sport),
			"data":    data,
		},
	}

	client := resty.New()
	_, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", "apikey "+apiKey).
		SetBody(payload).
		Post(apiUrl)

	return err
}

func (s *Server) getPricesBySport(sportName string) ([]models.Price, error) {
	var prices []models.Price
	err := db.DB.Preload("Market.MarketCollection.Event.Competition.Country.Sport").
		Joins("JOIN markets ON prices.market_id = markets.id").
		Joins("JOIN market_collections ON markets.market_collection_id = market_collections.id").
		Joins("JOIN events ON market_collections.event_id = events.id").
		Joins("JOIN competitions ON events.competition_id = competitions.id").
		Joins("JOIN countries ON competitions.country_id = countries.id").
		Joins("JOIN sports ON countries.sport_id = sports.id").
		Where("sports.name = ? AND prices.active = ? AND prices.status = ?", sportName, true, "active").
		Find(&prices).Error
	return prices, err
}
