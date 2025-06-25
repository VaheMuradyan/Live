package server

import (
	"fmt"
	"github.com/VaheMuradyan/Live/db"
	"github.com/VaheMuradyan/Live/db/models"
	"log"
	"math/rand"
	"time"
)

func (s *Server) updateGoalsMarkets(sport, param string) error {
	if _, exists := s.sportRoutines.Load(sport + "_goals_stop"); exists {
		return nil
	}

	overPrices, err := s.getPricesBySport(sport, "OVER_"+param, false)
	if err != nil {
		return fmt.Errorf("error getting GOALS prices for %s: %v", sport, err)
	}
	underPrices, err := s.getPricesBySport(sport, "UNDER_"+param, false)
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

func (s *Server) handleGoalsMarketLifecycle(sportName string) {
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
				delay = 30 * time.Second
			case "35":
				delay = 60 * time.Second
			case "45":
				delay = 90 * time.Second
			}

			go func(collectionID uint, argument string, d time.Duration) {
				time.Sleep(d)
				s.removeGoalsArgument(collectionID, argument)
			}(colID, argCopy, delay)
		}
	}

	go func() {
		time.Sleep(95 * time.Second)
		s.sportRoutines.Store(sportName+"_goals_stop", true)
	}()
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
