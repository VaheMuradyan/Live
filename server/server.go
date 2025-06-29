package server

import (
	"context"
	"encoding/json"
	"fmt"
	apiproto "github.com/VaheMuradyan/Live/centrifugo"
	"github.com/VaheMuradyan/Live/db/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VaheMuradyan/Live/db"
	"github.com/VaheMuradyan/Live/proto"
)

const (
	apiKey = "0957bfe1-5aa9-40c0-991f-d15150f91594"
)

type Server struct {
	live.UnimplementedCoefficientServiceServer
	stopRoutines  sync.Map
	sportRoutines sync.Map
	cfConn        *grpc.ClientConn
	cfClient      apiproto.CentrifugoApiClient
	counter       sync.Map
}

func NewServer() *Server {

	conn, err := grpc.Dial("localhost:10000", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Centrifugo: %v", err)
	}

	client := apiproto.NewCentrifugoApiClient(conn)

	server := &Server{
		cfConn:        conn,
		cfClient:      client,
		stopRoutines:  sync.Map{},
		sportRoutines: sync.Map{},
		counter:       sync.Map{},
	}

	sports := []string{"Football", "Basketball", "Tennis"}
	for _, sport := range sports {
		server.sportRoutines.Store(sport, make(chan bool))
	}
	return server
}

func (s *Server) StartSportUpdates(ctx context.Context, req *live.SportRequest) (*live.SportResponse, error) {
	sport := req.Sport
	interval := req.UpdateIntervalSeconds
	value, _ := s.sportRoutines.Load(sport)
	channel, _ := value.(chan bool)

	initial := &atomic.Int32{}
	initial.Store(5)
	s.counter.Store(sport, initial)

	if _, exists := s.stopRoutines.Load(sport); exists {
		return &live.SportResponse{
			Success: false,
			Message: fmt.Sprintf("Sport %s updates already running", sport),
			Sport:   sport,
		}, nil
	}

	stopChan := make(chan bool)
	s.stopRoutines.Store(sport, stopChan)

	goalsChan := make(chan bool)

	s.handleGoalsMarketLifecycle(sport, goalsChan)

	go func(ticker *time.Ticker, channel2 chan bool, sportName string, stopChan2 chan bool, sendGoals chan bool) {
		defer func() {
			ticker.Stop()
			close(sendGoals)
			s.stopRoutines.Delete(sport)
			s.counter.Delete(sport)
		}()
		for {
			select {
			case <-stopChan2:
				sendGoals <- false
				fmt.Println("Goroutine stopped")
				return
			case <-channel2:
				sendGoals <- true
				if err := s.generateCoefficientUpdate(sportName, true); err != nil {
					log.Printf("Error updating coefficient for %s: %v", sport, err)
				}
			case <-ticker.C:
				if err := s.generateCoefficientUpdate(sportName, false); err != nil {
					log.Printf("Error updating coefficient for %s: %v", sport, err)
				}
			}
		}
	}(time.NewTicker(time.Duration(interval)*time.Second), channel, sport, stopChan, goalsChan)

	return &live.SportResponse{
		Success: true,
		Message: fmt.Sprintf("Started %s coefficient updates", sport),
		Sport:   sport,
	}, nil
}

func (s *Server) StartEvents(ctx context.Context, req *live.EventRequest) (*live.EventResponse, error) {
	event := req.Event
	interval := req.ScoreUpdateTime
	sportName := req.SportName

	value, _ := s.sportRoutines.Load(sportName)
	channel, _ := value.(chan bool)

	stopChan := make(chan bool, 1)
	s.stopRoutines.Store(event, stopChan)
	s.handleScoreForEvent(event)

	go func(ticker *time.Ticker, eventName string, sportName2 string, stopChan2 chan bool, channel2 chan bool) {
		defer func() {
			ticker.Stop()
			s.stopRoutines.Delete(eventName)
		}()
		for {
			select {
			case <-stopChan2:
				end2, ok := s.stopRoutines.Load(sportName2)
				if ok {
					if end, ok2 := end2.(chan bool); ok2 {
						end <- true
					}
				}
				fmt.Println("Event stopped")
				return
			case <-ticker.C:
				if err := s.startEvent(eventName, channel2); err != nil {
					log.Printf("Error updating score for %s: %v", eventName, err)
				}
			}
		}
	}(time.NewTicker(time.Duration(interval)*time.Second), event, sportName, stopChan, channel)

	go func(eventName string) {
		time.Sleep(80 * time.Second)
		end2, ok := s.stopRoutines.Load(eventName)
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

func (s *Server) generateCoefficientUpdate(sport string, flag bool) error {
	var wg sync.WaitGroup
	wg.Add(5)

	if flag {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.updateMainMarkets(sport); err != nil {
				log.Printf("Error updating MAIN markets for %s: %v", sport, err)
			}
		}()
	}

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
	md := metadata.New(map[string]string{
		"authorization": "apikey " + apiKey,
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

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

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req := &apiproto.PublishRequest{
		Channel: channelName,
		Data:    jsonData,
	}

	_, err = s.cfClient.Publish(ctx, req)

	return err
}

func (s *Server) startEvent(eventName string, channel chan bool) error {
	md := metadata.New(map[string]string{
		"authorization": "apikey " + apiKey,
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

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
	if score.Event.Name == "" {
		err = db.DB.Preload("Event").First(&score, score.ID).Error
		if err != nil {
			log.Printf("Error preloading event for score ID %d: %v", score.ID, err)
			return err
		}
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

	data := map[string]interface{}{
		"game":   score.Event.Name,
		"score1": score.Team1Score,
		"score2": score.Team2Score,
		"total":  score.Total,
	}

	channelName := strings.ReplaceAll(score.Event.Name, " ", "")

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req := &apiproto.PublishRequest{
		Channel: channelName,
		Data:    jsonData,
	}

	_, err = s.cfClient.Publish(ctx, req)

	channel <- true

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
