package models

import (
	"gorm.io/gorm"
	"time"
)

type Country struct {
	gorm.Model
	Name         string        `json:"name"`
	Code         string        `gorm:"unique;size:3" json:"code"`
	SportID      uint          `json:"sport_id"`
	Sport        Sport         `gorm:"foreignKey:SportID" json:"sport,omitempty"`
	Competitions []Competition `gorm:"foreignKey:CountryID" json:"competitions,omitempty"`
}

type Competition struct {
	gorm.Model
	Name      string  `json:"name"`
	CountryID uint    `json:"country_id"`
	Country   Country `gorm:"foreignKey:CountryID" json:"country,omitempty"`
	Teams     []Team  `gorm:"many2many:competition_teams;" json:"teams,omitempty"`
	Events    []Event `gorm:"foreignKey:CompetitionID" json:"events,omitempty"`
}

type Event struct {
	gorm.Model
	Name              string             `json:"name"`
	CompetitionID     uint               `json:"competition_id"`
	Competition       Competition        `gorm:"foreignKey:CompetitionID" json:"competition,omitempty"`
	MarketCollections []MarketCollection `gorm:"foreignKey:EventID" json:"market_collections,omitempty"`
	Teams             []Team             `gorm:"many2many:event_teams;" json:"teams,omitempty"`
	StartTime         time.Time          `json:"start_time"`
}

type MarketCollection struct {
	gorm.Model
	Name    string   `json:"name"`
	Code    string   `json:"code"`
	EventID uint     `json:"event_id"`
	Event   Event    `gorm:"foreignKey:EventID" json:"event,omitempty"`
	Markets []Market `gorm:"foreignKey:MarketCollectionID" json:"markets,omitempty"`
}

type Market struct {
	gorm.Model
	Name               string           `json:"name"`
	Code               string           `json:"code"`
	Type               string           `json:"type"`
	MarketCollectionID uint             `json:"market_collection_id"`
	MarketCollection   MarketCollection `gorm:"foreignKey:MarketCollectionID" json:"market_collection,omitempty"`
	Prices             []Price          `gorm:"foreignKey:MarketID" json:"prices,omitempty"`
	LastUpdated        time.Time        `json:"last_updated"`
}

type Price struct {
	gorm.Model
	Name                string        `json:"name"`
	Code                string        `json:"code"`
	MarketID            uint          `json:"market_id"`
	Market              Market        `gorm:"foreignKey:MarketID" json:"market,omitempty"`
	CurrentCoefficient  float64       `gorm:"type:decimal(9,4);" json:"current_coefficient"`
	PreviousCoefficient float64       `gorm:"type:decimal(9,4);" json:"previous_coefficient"`
	Status              string        `gorm:"default:'active'" json:"status"`
	Active              bool          `gorm:"default:true" json:"active"`
	Coefficients        []Coefficient `gorm:"foreignKey:PriceID" json:"coefficients,omitempty"`
	LastUpdated         time.Time     `json:"last_updated"`
}

type Coefficient struct {
	gorm.Model
	PriceID uint    `json:"price_id"`
	Price   Price   `gorm:"foreignKey:PriceID" json:"price,omitempty"`
	Value   float64 `gorm:"type:decimal(9,4)" json:"value"`
}
type Team struct {
	gorm.Model
	Name         string        `json:"name"`
	Rating       int           `json:"rating"`
	CountryID    uint          `json:"country_id"`
	Country      Country       `gorm:"foreignKey:CountryID" json:"country,omitempty"`
	Competitions []Competition `gorm:"many2many:competition_teams;" json:"competitions,omitempty"`
	Events       []Event       `gorm:"many2many:event_teams;" json:"events,omitempty"`
	Sports       []Sport       `gorm:"many2many:sport_teams;" json:"sports,omitempty"`
}

type Sport struct {
	gorm.Model
	Name      string    `json:"name"`
	Code      string    `gorm:"unique" json:"code"`
	Countries []Country `gorm:"foreignKey:SportID" json:"countries,omitempty"`
	Teams     []Team    `gorm:"many2many:sport_teams;" json:"teams,omitempty"`
}
