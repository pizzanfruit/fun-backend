package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo"
	"github.com/mitchellh/mapstructure"
)

type (
	command struct {
		Type    string                 `json:"type" validate:"required,max=20"`
		Payload map[string]interface{} `json:"payload"`
	}
	// Response smt
	Response struct {
		Type    string      `json:"type" validate:"required"`
		Payload interface{} `json:"payload,omitempty"`
	}
)

const (

	// Maximum message size allowed from peer.
	maxMessageSize = 8192

	// Time allowed to read the next pong message from the peer.
	pongWait = 30 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{}

func websocketHandler(c echo.Context) error {
	upgrader.CheckOrigin = customCheckOrigin
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		c.Logger().Debug(err)
		return err
	}
	defer ws.Close()
	// Ping
	done := make(chan struct{})
	defer close(done)
	go ping(ws, done)
	// Welcome
	err = ws.WriteMessage(websocket.TextMessage, []byte("Please login!"))
	if err != nil {
		c.Logger().Error(err)
		return err
	}

	readPump(ws)
	return nil
}

func customCheckOrigin(r *http.Request) bool {
	return true
}

func ping(ws *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Println("ping:", err)
			}
		case <-done:
			return
		}
	}
}

func readPump(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	// Authenticate
	player := &Player{}
	err := ws.ReadJSON(player)
	if err != nil {
		log.Println("[Error] Invalid json.", err)
		ws.WriteMessage(websocket.CloseMessage, []byte("Invalid json"))
		return
	}
	playerRef := client.Collection("players").Doc(player.Name)
	playerSnap, err := playerRef.Get(context.Background())
	if err != nil {
		log.Println("[Error] Invalid player.", err)
		ws.WriteMessage(websocket.CloseMessage, []byte("Invalid player"))
		return
	}
	password := playerSnap.Data()["password"]
	if password != player.Password {
		log.Println("[Error] Invalid password.")
		ws.WriteMessage(websocket.CloseMessage, []byte("Invalid password"))
		return
	}
	// Logged in
	log.Printf("Player %s logged in!\n", player.Name)
	ws.WriteMessage(websocket.TextMessage, []byte("Logged in successfully!"))
	newPlayersChannels[player.Name] <- "logged in"
	// Log user out when connection is finished
	defer func() {
		log.Printf("Player %s logged out!\n", player.Name)
		playerRef.Delete(context.Background())
		leaveRoom(player.RoomID, player.Name)
	}()
	// Handle authenticated interactions
loop:
	for {
		command := &command{}
		err := ws.ReadJSON(command)
		if err != nil {
			log.Println("[Error] Invalid command.", err)
			break
		}
		log.Printf("Command: Type %s - Payload %s\n", command.Type, command.Payload)
		switch command.Type {
		case "create-room":
			// Get request payload
			room := &Room{}
			if err := mapstructure.Decode(command.Payload, &room); err != nil {
				log.Printf("[Error] Client %s - Invalid command payload. %s\n", player.Name, err)
				break loop
			}
			log.Printf("[Request] Client %s wants to create new room!\n", player.Name)
			// Create room
			id, err := createNewRoom(room)
			if err != nil {
				log.Printf("[Error] Client %s - Can't create new room. %s\n", player.Name, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Can't create new room"})
				break
			}
			ws.WriteJSON(Response{Type: "success", Payload: id})
		case "join-room":
			if player.RoomID != "" {
				if err := leaveRoom(player.RoomID, player.Name); err != nil {
					log.Printf("[Error] Client %s can't leave room %s. %s\n", player.Name, player.RoomID, err)
					ws.WriteJSON(Response{Type: "error", Payload: "Can't leave room"})
					break
				}
			}
			roomID, ok := command.Payload["id"].(string)
			if !ok {
				log.Printf("[Error] Client %s - Invalid command payload. %s\n", player.Name, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Invalid command payload"})
				break loop
			}
			log.Printf("[Request] Client %s wants to join room %s!\n", player.Name, roomID)
			// Join room
			if err := joinRoom(roomID, player.Name); err != nil {
				log.Printf("[Error] Client %s - Can't join room %s. %s\n", roomID, player.Name, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Can't join room"})
				break
			}
			player.RoomID = roomID
			ws.WriteJSON(Response{Type: "success"})
		case "leave-room":
			if player.RoomID == "" {
				log.Printf("[Error] Client %s - Not in a room. %s\n", player.Name, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Player not in a room"})
				break
			}
			log.Printf("[Request] Client %s wants to leave room %s!\n", player.Name, player.RoomID)
			// Leave room
			if err := leaveRoom(player.RoomID, player.Name); err != nil {
				log.Printf("[Error] Player %s can't leave room %s. %s\n", player.RoomID, player.Name, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Can't leave room"})
				break
			}
			player.RoomID = ""
			ws.WriteJSON(Response{Type: "success"})
		case "change-status":
			statusCodeFloat, ok := command.Payload["statusCode"].(float64)
			if !ok {
				log.Printf("[Error] Client %s - Invalid command payload. %s\n", player.Name, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Invalid command payload"})
				break loop
			}
			statusCode := int(statusCodeFloat)
			log.Printf("[Request] Client %s wants to change status code to %d!\n", player.Name, statusCode)
			// Change status code
			if err := setPlayerStatusByName(player.Name, statusCode); err != nil {
				log.Printf("[Error] Client %s - Can't change status code to %d. %s\n", player.Name, statusCode, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Can't change status"})
				break
			}
			ws.WriteJSON(Response{Type: "success"})
		case "chat-in-room":
			message, ok := command.Payload["message"].(string)
			if !ok {
				log.Printf("[Error] Client %s - Invalid chat message. %s\n", player.Name, err)
				break loop
			}
			log.Printf("[Request] Client %s wants to chat in room: %s!\n", player.Name, message)
			// Send message
			if err := chatInRoom(player, message); err != nil {
				log.Printf("[Error] Client %s - Can't send message to room. %s\n", player.Name, err)
				ws.WriteJSON(Response{Type: "error", Payload: "Can't send message"})
				break
			}
			ws.WriteJSON(Response{Type: "success"})
		default:
			break
		}
	}
}
