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
	return &Server{
		sportRoutines: sync.Map{},
	}
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

	s.handleGoalsMarketLifecycle(sport)

	go func(ticker *time.Ticker, sportName string, stopChan2 chan bool) {
		defer ticker.Stop()
		for {
			select {
			case <-stopChan2:
				fmt.Println("Goroutine stopped")
				return
			case <-ticker.C:
				if err := s.generateCoefficientUpdate(sportName); err != nil {
					log.Printf("Error updating coefficient for %s: %v", sport, err)
				}
			}
		}
	}(time.NewTicker(time.Duration(interval)*time.Second), sport, stopChan)

	go func(sportName string) {
		time.Sleep(160 * time.Second)
		end2, ok := s.sportRoutines.Load(sportName)
		if ok {
			if end, ok2 := end2.(chan bool); ok2 {
				end <- true
			}
		}
	}(sport)

	return &live.SportResponse{
		Success: true,
		Message: fmt.Sprintf("Started %s coefficient updates", sport),
		Sport:   sport,
	}, nil
}

func (s *Server) StartEvents(ctx context.Context, req *live.EventRequest) (*live.EventResponse, error) {
	event := req.Event
	interval := req.ScoreUpdateTime

	stopChan := make(chan bool)
	s.sportRoutines.Store(event, stopChan)
	s.handleScoreForEvent(event)

	go func(ticker *time.Ticker, eventName string, stopChan2 chan bool) {
		defer ticker.Stop()
		for {
			select {
			case <-stopChan2:
				fmt.Println("Event stopped")
				return
			case <-ticker.C:
				if err := s.startEvent(eventName); err != nil {
					log.Printf("Error updating score for %s: %v", eventName, err)
				}
			}
		}
	}(time.NewTicker(time.Duration(interval)*time.Second), event, stopChan)

	go func(eventName string) {
		time.Sleep(160 * time.Second)
		end2, ok := s.sportRoutines.Load(eventName)
		if ok {
			if end, ok2 := end2.(chan bool); ok2 {
				end <- true
			}
		}
	}(event)

	return &live.EventResponse{
		Success: true,
	}, nil
}

func (s *Server) generateCoefficientUpdate(sport string) error {
	var wg sync.WaitGroup
	wg.Add(6)

	go func() {
		defer wg.Done()
		if err := s.updateMainMarkets(sport); err != nil {
			log.Printf("Error updating MAIN markets for %s: %v", sport, err)
		}
	}()

	params := []string{"[0.5]", "[1.5]", "[2.5]", "[3.5]", "[4.5]"}

	for _, param := range params {
		go func(p string) {
			defer wg.Done()
			if err := s.updateGoalsMarkets(sport, p); err != nil {
				log.Printf("Error updating goals for %s: %v", sport, err)
			}
		}(param)
	}

	wg.Wait()
	return nil
}

func (s *Server) updateCoefficient(price *models.Price) error {
	//score, err := s.getScore(price)
	//if err != nil {
	//	return err
	//}

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
		"market":                 price.Code,
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

func (s *Server) startEvent(eventName string) error {
	var score models.Score

	query := `
	SELECT scores.*
	FROM scores
	JOIN events ON events.id=scores.event_id
	WHERE events.name = ?
	`

	err := db.DB.Raw(query, eventName).Scan(&score).Error
	if err != nil {
		log.Printf("Error getting score for %s: %v", eventName, err)
		return err
	}

	currentScoreTeam1 := score.Team1Score
	currentScoreTeam2 := score.Team2Score
	total := score.Total

	rand.Seed(time.Now().UnixNano())

	x := rand.Intn(2)

	switch x {
	case 0:
		currentScoreTeam1++
		score.Team1Score = currentScoreTeam1
		total++
	case 1:
		currentScoreTeam2++
		score.Team2Score = currentScoreTeam2
		total++
	}
	score.Total = total

	err = db.DB.Save(&score).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *Server) handleScoreForEvent(eventName string) {
	var score models.Score

	query := `
	SELECT scores.*
	FROM scores
	JOIN events ON events.id = scores.event_id
	WHERE events.name = ?
	`

	err := db.DB.Raw(query, eventName).Scan(&score).Error
	if err != nil {
		log.Printf("Error getting score for %s: %v", eventName, err)
		return
	}

	score.Team1Score = 0
	score.Team2Score = 0
	score.Total = 0

	err = db.DB.Save(&score).Error
	if err != nil {
		log.Printf("Error updating score ID %d: %v", score.ID, err)
	}
}

//TODO petqa querinerov prost@ grvi vor karenam haskanam vonca taeq@ hendl anelu

func (s *Server) getPricesBySport(sportName string, filterValue string, flag bool) ([]models.Price, error) {
	var prices []models.Price

	query := db.DB.Preload("Market.MarketCollection.Event.Competition.Country.Sport").
		Joins("JOIN markets ON prices.market_id = markets.id").
		Joins("JOIN market_collections ON markets.market_collection_id = market_collections.id").
		Joins("JOIN events ON market_collections.event_id = events.id").
		Joins("JOIN competitions ON events.competition_id = competitions.id").
		Joins("JOIN countries ON competitions.country_id = countries.id").
		Joins("JOIN sports ON countries.sport_id = sports.id").
		Where("sports.name = ? AND prices.active = ?", sportName, true)

	if flag {
		query = query.Where("market_collections.code = ?", filterValue)
	} else {
		query = query.Where("prices.code = ?", filterValue)
	}

	err := query.Find(&prices).Error
	return prices, err
}

func (s *Server) getScore(price *models.Price) (*models.Score, error) {
	var score models.Score

	err := db.DB.Table("scores").
		Joins("JOIN events ON scores.event_id = events.id").
		Joins("JOIN market_collections ON events.id = market_collections.event_id").
		Joins("JOIN markets ON market_collections.id = markets.market_collection_id").
		Where("markets.id = ?", price.MarketID).
		First(&score).Error

	if err != nil {
		return nil, err
	}

	return &score, nil
}
