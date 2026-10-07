package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Client and Project live in SQLite and are served by contrib/gorm, unlike
// the in-memory repositories in models.go. Client has no has-many back to
// its projects, so deleting one still referenced reaches the database,
// which refuses it.
type Client struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:100;uniqueIndex;not null"`
}

type Project struct {
	ID       uint       `gorm:"primaryKey"`
	Name     string     `gorm:"size:100;uniqueIndex;not null"`
	Status   string     `gorm:"size:20;not null"`
	Active   bool       `gorm:"not null"`
	Due      *time.Time `gorm:"type:date"`
	ClientID *uint
	Client   *Client `gorm:"constraint:OnDelete:RESTRICT"`
}

var projectsDBCount atomic.Int64

func day(year int, month time.Month, d int) *time.Time {
	t := time.Date(year, month, d, 0, 0, 0, 0, time.Local)
	return &t
}

// openProjectsDB opens a fresh in-memory database, named per call so test
// apps never share one, and seeds it.
func openProjectsDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf("file:projects-%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", projectsDBCount.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&Client{}, &Project{}); err != nil {
		return nil, err
	}
	clients := []*Client{{Name: "Northwind"}, {Name: "Contoso"}, {Name: "Umbrella"}}
	for _, c := range clients {
		if err := db.Create(c).Error; err != nil {
			return nil, err
		}
	}
	projects := []Project{
		{Name: "Apollo", Status: "active", Active: true, Due: day(2026, 11, 1), ClientID: &clients[0].ID},
		{Name: "Borealis", Status: "planned", Active: true, Due: day(2026, 12, 15), ClientID: &clients[0].ID},
		{Name: "Cascade", Status: "done", Active: false, Due: day(2026, 3, 1), ClientID: &clients[1].ID},
		{Name: "Delta", Status: "active", Active: true, ClientID: &clients[1].ID},
		{Name: "Eclipse", Status: "planned", Active: true, Due: day(2027, 1, 10), ClientID: &clients[2].ID},
		{Name: "Fjord", Status: "planned", Active: true},
	}
	for i := range projects {
		if err := db.Create(&projects[i]).Error; err != nil {
			return nil, err
		}
	}
	return db, nil
}
