package server

import (
	"fmt"
	"github.com/VaheMuradyan/Live/db"
	"github.com/VaheMuradyan/Live/db/models"
	"log"
	"math/rand"
	"sync/atomic"
)

func (s *Server) updateGoalsMarkets(sport, param string) error {
	overPrices, err := s.getPricesBySport(sport, "OVER"+param, false)
	if err != nil {
		return fmt.Errorf("error getting GOALS prices for %s: %v", sport, err)
	}
	underPrices, err := s.getPricesBySport(sport, "UNDER"+param, false)
	if err != nil {
		return fmt.Errorf("error getting GOALS prices for %s: %v", sport, err)
	}
	allPrices := append(overPrices, underPrices...)

	if len(allPrices) == 0 {
		return fmt.Errorf("no active GOALS prices found for sport %s", sport)
	}

	randomPrice := allPrices[rand.Intn(len(allPrices))]
	return s.updateCoefficient(&randomPrice)
}

func (s *Server) handleGoalsMarketLifecycle(sportName string, goalsChan chan bool) {
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
		s.activateGoalsMarketWithArgument(col.ID, "5")
		s.activateGoalsMarketWithArgument(col.ID, "15")
		s.activateGoalsMarketWithArgument(col.ID, "25")
		s.activateGoalsMarketWithArgument(col.ID, "35")
		s.activateGoalsMarketWithArgument(col.ID, "45")
	}

	go s.listen(sportName, goalsChan, collections)
}

func (s *Server) listen(sportName string, goalsChan chan bool, collections []models.MarketCollection) {
	for {
		select {
		case val, ok := <-goalsChan:
			if !ok || !val {
				return
			}

			x, _ := s.counter.Load(sportName)
			xv := x.(*atomic.Int32)
			if xv.Load() > 45 {
				return
			}
			sx := fmt.Sprintf("%v", xv.Load())
			for _, col := range collections {
				s.removeGoalsArgument(col.ID, sx)
			}
			xv.Add(10)
		}
	}
}

func (s *Server) activateGoalsMarketWithArgument(collectionId uint, argument string) {
	arg := fmt.Sprintf("OU%s", argument)

	var market models.Market
	result := db.DB.Where("market_collection_id = ? AND code = ?",
		collectionId, arg).First(&market)

	if result.Error == nil {
		db.DB.Model(&models.Price{}).Where("market_id = ?", market.ID).Update("active", true)
		return
	}
}

func (s *Server) removeGoalsArgument(collectionID uint, argument string) {
	arg := fmt.Sprintf("OU%s", argument)

	var market models.Market
	if err := db.DB.Where("market_collection_id = ? AND code = ?", collectionID, arg).First(&market).Error; err != nil {
		return
	}

	db.DB.Model(&models.Price{}).Where("market_id = ?", market.ID).Update("active", false)
}
