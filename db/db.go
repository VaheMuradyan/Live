package db

import (
	"github.com/VaheMuradyan/Live/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"time"
)

var DB *gorm.DB

func Connect() {
	var err error
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Info,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)
	dsn := "vahe:java@tcp(127.0.0.1:3306)/sport?charset=utf8mb4&parseTime=True&loc=Local"
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: newLogger})

	if err != nil {
		panic("Failed to connect to databse!")
	}

	err = DB.AutoMigrate(&models.Sport{}, &models.Country{}, &models.Competition{}, &models.Team{}, &models.Event{},
		&models.MarketCollection{}, &models.Market{}, &models.Price{}, &models.Coefficient{})
	if err != nil {
		log.Fatal("Failed to migrate database!")
	}
}
