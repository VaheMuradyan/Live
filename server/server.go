package server

import (
	"context"
	"fmt"
	"github.com/VaheMuradyan/Live/db/models"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/VaheMuradyan/Live/db"
	"github.com/VaheMuradyan/Live/proto"
	"github.com/go-resty/resty/v2"
)

const (
	apiKey = "0957bfe1-5aa9-40c0-991f-d15150f91594"
	apiUrl = "http://localhost:8000/api"
)

type Server struct {
	live.UnimplementedCoefficientServiceServer
	sportRoutines sync.Map
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) StartSportUpdates(ctx context.Context, req *live.SportRequest) (*live.SportResponse, error) {
	sport := req.Sport
	interval := req.UpdateIntervalSeconds

	if _, exists := s.sportRoutines.Load(sport); exists {
		return &live.SportResponse{
			Success: false,
			Message: fmt.Sprintf("Sport %s updates already running", sport),
			Sport:   sport,
		}, nil
	}

	stopChan := make(chan bool)
	s.sportRoutines.Store(sport, stopChan)

	s.expandTotalGoalsMarkets(sport)

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

func (s *Server) generateCoefficientUpdate(sport string) error {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := s.updateMainMarkets(sport); err != nil {
			log.Printf("Error updating MAIN markets for %s: %v", sport, err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := s.updateGoalsMarkets(sport); err != nil {
			log.Println("END")
		}
	}()

	wg.Wait()
	return nil
}

func (s *Server) updateMainMarkets(sport string) error {
	prices, err := s.getPricesBySport(sport, "MAIN")
	if err != nil {
		return fmt.Errorf("error getting MAIN prices for %s: %v", sport, err)
	}

	if len(prices) == 0 {
		return fmt.Errorf("no active MAIN prices found for sport %s", sport)
	}

	randomPrice := prices[rand.Intn(len(prices))]
	return s.updateCoefficient(&randomPrice)
}

func (s *Server) updateGoalsMarkets(sport string) error {
	if _, exists := s.sportRoutines.Load(sport + "_goals_stop"); exists {
		return nil
	}

	prices, err := s.getPricesBySport(sport, "GOALS")
	if err != nil {
		return fmt.Errorf("error getting GOALS prices for %s: %v", sport, err)
	}

	if len(prices) == 0 {
		return fmt.Errorf("no active GOALS prices found for sport %s", sport)
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

	result := db.DB.Model(&models.Price{}).
		Where("id = ? AND active = ?", price.ID, true).
		Updates(map[string]interface{}{
			"previous_coefficient": oldCoeff,
			"current_coefficient":  newCoeff,
			"last_updated":         time.Now(),
		})

	if result.RowsAffected == 0 {
		return nil
	}

	price.PreviousCoefficient = oldCoeff
	price.CurrentCoefficient = newCoeff
	price.LastUpdated = time.Now()

	return s.sendToCentrifugo(price)
}

func (s *Server) sendToCentrifugo(price *models.Price) error {
	sport := price.Market.MarketCollection.Event.Competition.Country.Sport.Name
	marketCollectionCode := price.Market.MarketCollection.Code
	data := map[string]interface{}{
		"sport":                  sport,
		"country":                price.Market.MarketCollection.Event.Competition.Country.Name,
		"competition":            price.Market.MarketCollection.Event.Competition.Name,
		"event":                  price.Market.MarketCollection.Event.Name,
		"market":                 price.Market.Name,
		"market_type":            price.Market.Type,
		"market_collection_code": price.Market.MarketCollection.Code,
		"price":                  price.Name,
		"old_coefficient":        float32(price.PreviousCoefficient),
		"new_coefficient":        float32(price.CurrentCoefficient),
		"timestamp":              time.Now().Format(time.RFC3339),
		"change":                 float32(price.CurrentCoefficient) - float32(price.PreviousCoefficient),
	}

	channelName := strings.ToLower(sport) + "_" + strings.ToLower(marketCollectionCode)

	payload := map[string]interface{}{
		"method": "publish",
		"params": map[string]interface{}{
			"channel": channelName,
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

func (s *Server) expandTotalGoalsMarkets(sportName string) {
	var collections []models.MarketCollection
	err := db.DB.Preload("Event.Competition.Country.Sport").
		Joins("JOIN events ON market_collections.event_id = events.id").
		Joins("JOIN competitions ON events.competition_id = competitions.id").
		Joins("JOIN countries ON competitions.country_id = countries.id").
		Joins("JOIN sports ON countries.sport_id = sports.id").
		Where("sports.name = ? AND market_collections.code = ?", sportName, "GOALS").
		Find(&collections).Error

	if err != nil {
		log.Printf("Error getting goals collections for sport %s: %v", sportName, err)
		return
	}

	for _, col := range collections {
		s.activateGoalsMarketWithArgument(col.ID, "25")
		s.activateGoalsMarketWithArgument(col.ID, "35")
		s.activateGoalsMarketWithArgument(col.ID, "45")
	}

	for _, col := range collections {
		for _, arg := range []string{"25", "35", "45"} {
			argCopy := arg
			colID := col.ID

			var delay time.Duration
			switch argCopy {
			case "25":
				delay = 20 * time.Second
			case "35":
				delay = 30 * time.Second
			case "45":
				delay = 40 * time.Second
			}

			go func(collectionID uint, argument string, d time.Duration) {
				time.Sleep(d)
				s.removeGoalsArgument(collectionID, argument)
			}(colID, argCopy, delay)
		}
	}

	go func() {
		time.Sleep(45 * time.Second)
		s.sportRoutines.Store(sportName+"_goals_stop", true)
	}()
}

func (s *Server) activateGoalsMarketWithArgument(collectionId uint, argument string) {
	arg := fmt.Sprintf("OU%s", argument)
	var existingMarket models.Market
	result := db.DB.Where("market_collection_id = ? AND code = ?",
		collectionId, arg).First(&existingMarket)

	if result.Error == nil {
		db.DB.Model(&models.Price{}).Where("market_id = ?", existingMarket.ID).
			Update("active", true)
		return
	}
}

func (s *Server) removeGoalsArgument(collectionID uint, argument string) {
	arg := fmt.Sprintf("OU%s", argument)

	var market models.Market
	if err := db.DB.Where("market_collection_id = ? AND code = ?",
		collectionID, arg).First(&market).Error; err != nil {
		return
	}

	db.DB.Model(&models.Price{}).Where("market_id = ?", market.ID).
		Update("inactive", false)
}

func (s *Server) getPricesBySport(sportName string, code string) ([]models.Price, error) {
	var prices []models.Price
	err := db.DB.Preload("Market.MarketCollection.Event.Competition.Country.Sport").
		Joins("JOIN markets ON prices.market_id = markets.id").
		Joins("JOIN market_collections ON markets.market_collection_id = market_collections.id").
		Joins("JOIN events ON market_collections.event_id = events.id").
		Joins("JOIN competitions ON events.competition_id = competitions.id").
		Joins("JOIN countries ON competitions.country_id = countries.id").
		Joins("JOIN sports ON countries.sport_id = sports.id").
		Where("sports.name = ? AND prices.active = ? AND prices.status = ? AND market_collections.code = ?",
			sportName, true, "active", code).
		Find(&prices).Error
	return prices, err
}
