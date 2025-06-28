package models

import (
	"gorm.io/gorm"
	"time"
)

type Country struct {
	gorm.Model
	Name         string
	Code         string `gorm:"unique;size:3"`
	SportID      uint
	Sport        Sport         `gorm:"foreignKey:SportID"`
	Competitions []Competition `gorm:"foreignKey:CountryID"`
}

type Competition struct {
	gorm.Model
	Name      string
	CountryID uint
	Country   Country `gorm:"foreignKey:CountryID"`
	Teams     []Team  `gorm:"many2many:competition_teams;"`
	Events    []Event `gorm:"foreignKey:CompetitionID"`
}

type Event struct {
	gorm.Model
	Name              string `gorm:"unique;size:255"`
	CompetitionID     uint
	Competition       Competition        `gorm:"foreignKey:CompetitionID"`
	MarketCollections []MarketCollection `gorm:"foreignKey:EventID"`
	Teams             []Team             `gorm:"many2many:event_teams;"`
	StartTime         time.Time
}

type MarketCollection struct {
	gorm.Model
	Name    string
	Code    string
	EventID uint
	Event   Event    `gorm:"foreignKey:EventID"`
	Markets []Market `gorm:"foreignKey:MarketCollectionID"`
}

type Market struct {
	gorm.Model
	Name               string
	Code               string
	Type               string
	MarketCollectionID uint
	MarketCollection   MarketCollection `gorm:"foreignKey:MarketCollectionID" `
	Prices             []Price          `gorm:"foreignKey:MarketID"`
	LastUpdated        time.Time
}

type Price struct {
	gorm.Model
	Name                string
	Code                string
	MarketID            uint
	Market              Market  `gorm:"foreignKey:MarketID"`
	CurrentCoefficient  float64 `gorm:"type:decimal(9,4);"`
	PreviousCoefficient float64 `gorm:"type:decimal(9,4);"`
	Active              bool    `gorm:"default:true"`
	LastUpdated         time.Time
}
type Team struct {
	gorm.Model
	Name         string
	Rating       int
	CountryID    uint
	Country      Country       `gorm:"foreignKey:CountryID"`
	Competitions []Competition `gorm:"many2many:competition_teams;"`
	Events       []Event       `gorm:"many2many:event_teams;"`
	Sports       []Sport       `gorm:"many2many:sport_teams;"`
}

type Sport struct {
	gorm.Model
	Name      string
	Code      string    `gorm:"unique"`
	Countries []Country `gorm:"foreignKey:SportID"`
	Teams     []Team    `gorm:"many2many:sport_teams;"`
}

type Score struct {
	ID         uint `gorm:"primaryKey;autoIncrement"`
	EventID    uint
	Event      Event `gorm:"foreignKey:EventID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	Team1Score int
	Team2Score int
	Total      int
}
