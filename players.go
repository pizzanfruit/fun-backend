package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/labstack/echo"
	"github.com/sethvargo/go-password/password"
)

type (
	// Player smt
	Player struct {
		Name     string `json:"name" validate:"required,max=20"`
		Password string `json:"password" validate:"max=20"`
		RoomID   string `json:"roomId"`
	}
)

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
	newPlayerRef := client.Collection("players").Doc(player.Name)

	if _, err = newPlayerRef.Create(context.Background(), map[string]interface{}{
		"password":  newPassword,
		"createdAt": time.Now(),
	}); err != nil {
		log.Println(err)
		if strings.Contains(err.Error(), "AlreadyExists") {
			return c.String(http.StatusBadRequest, "AlreadyExists")
		}
		return
	}
	// Create a new go routine to delete created player if player doesn't connect
	ch := make(chan string)
	newPlayersChannels[player.Name] = ch
	go func() {
		for {
			select {
			case <-ch:
				log.Println("New player logged in before 2 minutes timeout:", player.Name)
				return
			case <-time.After(2 * time.Minute):
				log.Println("New player timed out after 2 minutes:", player.Name)
				deletePlayerByRef(newPlayerRef, player.Name)
				return
			}
		}
	}()
	player.Password = newPassword
	log.Println("Created new player:", player.Name)
	return c.JSON(http.StatusOK, player)
}

func deletePlayerByRef(ref *firestore.DocumentRef, name string) {
	if _, err := ref.Delete(context.Background()); err != nil {
		log.Println("Error deletePlayerByRef", err)
		return
	}
	log.Println("Deleted player:", name)
}

func setPlayerStatusByName(name string, statusCode int) error {
	playerRef := client.Collection("players").Doc(name)
	_, err := playerRef.Update(context.Background(), []firestore.Update{
		{Path: "statusCode", Value: statusCode},
	})
	if err != nil {
		return err
	}
	return nil
}
