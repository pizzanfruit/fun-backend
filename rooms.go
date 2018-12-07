package main

import (
	"context"
	"errors"
	"log"
	"time"

	"cloud.google.com/go/firestore"
)

type (
	// Room smt
	Room struct {
		Name       string
		MaxPlayers int
		Type       string
	}
)

func createNewRoom(room *Room) (string, error) {
	newRoomRef, _, err := client.Collection("rooms").Add(context.Background(), map[string]interface{}{
		"statusCode":  0,
		"playerByIds": []string{},
		"name":        room.Name,
		"maxPlayers":  room.MaxPlayers,
		"type":        room.Type,
		"createdAt":   time.Now(),
	})
	if err != nil {
		return "", err
	}
	newChatRef := client.Collection("chats").Doc(newRoomRef.ID)
	_, err = newChatRef.Create(context.Background(), map[string]interface{}{
		"messages":  []string{},
		"createdAt": time.Now(),
	})
	if err != nil {
		return "", err
	}
	return newRoomRef.ID, nil
}

func joinRoom(roomID string, playerName string) error {
	roomRef := client.Collection("rooms").Doc(roomID)
	// Check if room is full
	roomSnap, err := roomRef.Get(context.Background())
	if err != nil {
		return err
	}
	playerCount := len(roomSnap.Data()["playerByIds"].([]interface{}))
	maxPlayers64, ok := roomSnap.Data()["maxPlayers"].(int64)
	if !ok {
		return errors.New("Can't type assert maxPlayers")
	}
	maxPlayers := int(maxPlayers64)
	log.Println("playerCount", playerCount)
	log.Println("maxPlayers", maxPlayers)
	if playerCount < maxPlayers {
		_, err := roomRef.Update(context.Background(), []firestore.Update{
			{Path: "playerByIds", Value: firestore.ArrayUnion(playerName)},
		})
		if err != nil {
			return err
		}
		err = setPlayerStatusByName(playerName, 0)
		if err != nil {
			return err
		}
		return nil
	}
	return errors.New("Room already full")

}

func leaveRoom(roomID string, playerName string) error {
	if roomID == "" {
		log.Println("[Info] Player not in a room. No need to leave.")
		return nil
	}
	roomRef := client.Collection("rooms").Doc(roomID)
	chatRef := client.Collection("chats").Doc(roomID)
	// Delete player from room's player list
	_, err := roomRef.Update(context.Background(), []firestore.Update{
		{Path: "playerByIds", Value: firestore.ArrayRemove(playerName)},
	})
	if err != nil {
		return err
	}
	// Delete empty room
	roomSnap, err := roomRef.Get(context.Background())
	players := roomSnap.Data()["playerByIds"].([]interface{})
	if players == nil || len(players) == 0 {
		_, err = roomRef.Delete(context.Background())
		if err != nil {
			log.Println("[Error] Can't delete empty room.", err)
			return err
		}
		log.Println("Deleted empty room", roomID)
		_, err = chatRef.Delete(context.Background())
		if err != nil {
			log.Println("[Error] Can't delete chat of deleted room.", err)
			return err
		}
		log.Println("Deleted unused chat", roomID)
	}
	return nil
}

func chatInRoom(player *Player, message string) error {
	chatRef := client.Collection("chats").Doc(player.RoomID)
	newMessage := map[string]interface{}{"timestamp": time.Now(), "playerName": player.Name, "content": message}
	_, err := chatRef.Update(context.Background(), []firestore.Update{
		{Path: "messages", Value: firestore.ArrayUnion(newMessage)},
	})
	if err != nil {
		return err
	}
	return nil
}
