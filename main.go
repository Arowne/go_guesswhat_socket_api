package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strconv"

	gosocketio "github.com/graarh/golang-socketio"
	transport "github.com/graarh/golang-socketio/transport"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

// Room for database model
type Room struct {
	gorm.Model
	Name    string `json:"name"`
	Creator string `json:"creator"`
	Word    string `json:"word"`
}

// RoomUser for database model
type RoomUser struct {
	gorm.Model
	RoomID int    `json:"roomId"`
	Name   string `json:"name"`
	IP     string `json:"ip"`
}

func main() {

	type Message struct {
		Room    string `json:"room"`
		Name    string `json:"name"`
		Message string `json:"message"`
		Color   string `json:"color"`
		Find    bool   `json:"find"`
	}

	//SingleRoom
	type SingleRoom struct {
		Name    string `json:"name"`
		Creator string `json:"creator"`
	}

	//SingleRoom
	type JoinedRoom struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Creator  string `json:"creator"`
	}

	// Connect to database
	db, err := gorm.Open("sqlite3", "guesswhat.db")

	if err != nil {
		panic("failed to connect database")
	}

	defer db.Close()

	//create
	server := gosocketio.NewServer(transport.GetDefaultWebsocketTransport())

	//New connection
	server.On(gosocketio.OnConnection, func(c *gosocketio.Channel, args interface{}) {
		log.Println("New client connected")

		//join them to room
		c.Join("guesswhat")

		//or check the amount of clients in room
		amount := c.Amount("guesswhat")
		log.Println(amount, "actualy playing")

	})

	// Create room
	server.On("create-room", func(c *gosocketio.Channel, msg SingleRoom) **Room {

		token := sha256.Sum256([]byte(c.Ip()))
		getToken := fmt.Sprintf("%x", token)

		room := &Room{Name: msg.Name, Creator: getToken}

		db.Create(&room)

		return &room

	})

	//Join room
	server.On("join-room", func(c *gosocketio.Channel, joinedRoom JoinedRoom) string {

		//join them to room
		room := "chat-room/" + strconv.Itoa(joinedRoom.ID)

		c.Join(room)

		roomUser := &RoomUser{RoomID: joinedRoom.ID, Name: joinedRoom.Username, IP: c.Ip()}
		db.Create(&roomUser)

		userList := &[]RoomUser{}
		db.Where("room_id = ?", joinedRoom.ID).Find(&userList)

		c.BroadcastTo(room, "new-user", &userList)

		return room

	})

	// Get response
	server.On("response", func(c *gosocketio.Channel, msg Message) string {

		roomInfo := &Room{}
		db.Where("id = ?", msg.Room).Find(&roomInfo)

		room := "chat-room/" + msg.Room

		if roomInfo.Word == msg.Message {
			msg.Find = true
			c.BroadcastTo(room, "message", msg)
			return "Find"
		}
		c.BroadcastTo(room, "message", msg)

		return room

	})

	// Get room information
	server.On("get-room", func(c *gosocketio.Channel) **[]Room {
		rooms := &[]Room{}
		db.Find(&rooms)
		return &rooms
	})

	// Get room user list
	server.On("room-users", func(c *gosocketio.Channel, joinedRoom JoinedRoom) **[]RoomUser {
		userList := &[]RoomUser{}
		db.Where("room_id = ?", joinedRoom.ID).Find(&userList)
		return &userList
	})

	// Get turn
	server.On("get-turn", func(c *gosocketio.Channel, joinedRoom JoinedRoom) **[]RoomUser {
		userList := &[]RoomUser{}
		getRoomID := fmt.Sprintf("%d", joinedRoom.ID)
		getChanel := "chat-room/" + getRoomID

		db.Where("room_id = ?", joinedRoom.ID).Order(gorm.Expr("random()")).First(&userList)
		c.BroadcastTo(getChanel, "set-turn", &userList)

		return &userList
	})

	// Get set word
	server.On("set-word", func(c *gosocketio.Channel, message Message) *Message {
		room := &Room{}
		getRoomID := message.Room
		getChanel := "chat-room/" + getRoomID
		db.Where("id = ?", getRoomID).First(&room)
		room.Word = message.Message
		db.Save(&room)
		c.BroadcastTo(getChanel, "begin-turn", &room)
		return &message
	})

	// Get set word
	server.On("send-image", func(c *gosocketio.Channel, message Message) *Message {

		getRoomID := message.Room
		getChanel := "chat-room/" + getRoomID
		c.BroadcastTo(getChanel, "emit-draw", &message)
		return &message
	})

	// delete room
	server.On("delete-room", func(c *gosocketio.Channel, room Room) **[]Room {

		getRoomID := fmt.Sprintf("%d", room.ID)
		getChanel := "chat-room/" + getRoomID
		message := Message{}
		message.Name = "Creator"
		message.Message = "This room as been removed"

		c.BroadcastTo(getChanel, "quit-room", message)

		getRoom := &[]Room{}
		db.Where("id = ?", room.ID).Delete(&getRoom)
		return &getRoom

	})

	// Get response
	server.On("whoami", func(c *gosocketio.Channel, msg Message) **Room {

		roomInfo := &Room{}
		db.Where("id = ?", msg.Room).Find(&roomInfo)

		return &roomInfo

	})

	// Handle disconnection
	server.On(gosocketio.OnDisconnection, func(c *gosocketio.Channel) {
		//caller is not necessary, client will be removed from rooms
		//automatically on disconnect
		//but you can remove client from room whenever you need to
		c.Leave("guesswhat")

		userList := &[]RoomUser{}

		db.Where("ip = ?", c.Ip()).Delete(&userList)

		log.Println("Disconnected")

	})

	//setup http server
	serveMux := http.NewServeMux()
	serveMux.Handle("/socket.io/", server)
	log.Panic(http.ListenAndServe(":7000", serveMux))

}
