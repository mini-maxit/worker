package worker

import (
	"fmt"
	"log"

	"github.com/mini-maxit/worker/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type postgresDataBase struct {
	Db *gorm.DB
}

func Connect(db * postgresDataBase) *gorm.DB {
	return db.Db
}

// Connect to the database using GORM
func NewPostgresDatabase() *postgresDataBase {
	Config := LoadConfig()
	DATABASE_USER := Config.DBUser
	DATABASE_PASSWORD := Config.DBPassword
	DATABASE_NAME := Config.DBName
	DATABASE_SSL_MODE := Config.DBSslMode

	dsn := fmt.Sprintf("host=postgres user=%s password=%s dbname=%s sslmode=%s",
		DATABASE_USER, DATABASE_PASSWORD, DATABASE_NAME, DATABASE_SSL_MODE)


	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	//log the dsn to see if it is correct
	log.Println(dsn)
	utils.CheckError(err, "failed to connect to database")

	return &postgresDataBase{Db: db}
}
