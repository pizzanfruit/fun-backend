package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/sethvargo/go-password/password"
	validator "gopkg.in/go-playground/validator.v9"
)

var (
	client             *firestore.Client
	newPlayersChannels map[string]chan string
	upgrader           = websocket.Upgrader{}
)

func main() {
	projectID := "fpt-building"
	ctx := context.Background()
	newPlayersChannels = make(map[string]chan string)
	// Get a Firestore client.
	var err error
	client, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	// Close client when done.
	defer client.Close()

	if err != nil {
		log.Fatalf("Failed adding aturing: %v", err)
	}

	// Echo
	e := echo.New()
	e.Debug = true
	e.Validator = &CustomValidator{validator: validator.New()}

	e.Use(middleware.CORS())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "method=${method}, uri=${uri}, status=${status}\n",
	}))
	e.Use(middleware.Recover())

	e.GET("/hello", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello, World!")
	})
	e.GET("/ws", websocketHandler)

	// Players
	playersRoute := e.Group("/players")
	playersRoute.POST("/new", newPlayerHandler)

	e.Logger.Fatal(e.Start(":3000"))
}

// Validate smt
func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}

func websocketHandler(c echo.Context) error {
	upgrader.CheckOrigin = customCheckOrigin
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		c.Logger().Debug(err)
		return err
	}
	defer ws.Close()

	for {
		// Write
		err := ws.WriteMessage(websocket.TextMessage, []byte("Hello, Client!"))
		if err != nil {
			c.Logger().Error(err)
			return err
		}

		// Read
		_, msg, err := ws.ReadMessage()
		if err != nil {
			c.Logger().Error(err)
			return err
		}
		fmt.Printf("%s\n", msg)
	}
}

func customCheckOrigin(r *http.Request) bool {
	return true
}

func newPlayerHandler(c echo.Context) (err error) {
	player := new(Player)
	if err = c.Bind(player); err != nil {
		return
	}
	if err = c.Validate(player); err != nil {
		return
	}
	// Valid player
	// Generate a random password
	var newPassword string
	if newPassword, err = password.Generate(20, 5, 5, false, false); err != nil {
		return
	}
	// Create new player
	var playerRef *firestore.DocumentRef

	if playerRef, _, err = client.Collection("players").Add(context.Background(), map[string]interface{}{
		"name":      player.Name,
		"password":  newPassword,
		"createdAt": time.Now(),
	}); err != nil {
		return
	}
	// Create a new go routine to delete created player if player doesn't connect
	ch := make(chan string)
	newPlayersChannels[player.Name] = ch
	go func() {
		for {
			select {
			case <-ch:
				log.Println("New player timed out after 5 seconds:", player.Name)
				return
			case <-time.After(2 * time.Minute):
				deletePlayerByRef(playerRef, player.Name)
				return
			}
		}
	}()
	player.Password = newPassword
	return c.JSON(http.StatusOK, player)
}

func deletePlayerByRef(ref *firestore.DocumentRef, name string) {
	if _, err := ref.Delete(context.Background()); err != nil {
		log.Println("Error deletePlayerByRef", err)
		return
	}
	log.Println("Deleted player:", name)
}

type (
	// Player smt
	Player struct {
		Name     string `json:"name" validate:"required,max=20"`
		Password string `json:"password" validate:"max=20"`
	}

	// CustomValidator smt
	CustomValidator struct {
		validator *validator.Validate
	}
)
