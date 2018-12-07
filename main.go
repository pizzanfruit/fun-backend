package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	validator "gopkg.in/go-playground/validator.v9"
)

type (

	// CustomValidator smt
	CustomValidator struct {
		validator *validator.Validate
	}
)

var (
	client             *firestore.Client
	newPlayersChannels map[string]chan string
)

// Validate smt
func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}

func main() {
	port := os.Getenv("PORT")
	// Firestore init
	projectID := "fpt-building"
	ctx := context.Background()
	newPlayersChannels = make(map[string]chan string)
	var err error
	client, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()
	// Echo init
	e := echo.New()
	e.Debug = true
	e.Validator = &CustomValidator{validator: validator.New()}

	// Middleware
	e.Use(middleware.CORS())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "method=${method}, uri=${uri}, status=${status}\n",
	}))
	e.Use(middleware.Recover())

	// Routing
	e.GET("/hello", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.GET("/ws", websocketHandler)
	p := e.Group("/players")
	p.POST("", newPlayerHandler)
	// Close client when done.
	e.Logger.Fatal(e.Start(":" + port))
}
