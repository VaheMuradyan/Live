package server

import (
	"fmt"
)

func (s *Server) updateMainMarkets(sport string) error {
	prices, err := s.getPricesBySport(sport, "MAIN", true)
	if err != nil {
		return fmt.Errorf("error getting MAIN prices for %s: %v", sport, err)
	}

	if len(prices) == 0 {
		return fmt.Errorf("no active MAIN prices found for sport %s", sport)
	}

	_ = s.updateCoefficient(&prices[0])
	_ = s.updateCoefficient(&prices[1])
	return s.updateCoefficient(&prices[2])
}
