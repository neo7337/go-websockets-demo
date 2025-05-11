package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Message represents the structure of messages exchanged between client and server
type Message struct {
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Sender    string    `json:"sender"`
	Timestamp time.Time `json:"timestamp"`
}

// User represents a registered user
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"` // In a real app, this would be hashed
}

// Client represents a connected user
type Client struct {
	ID   string
	Conn *websocket.Conn
	Name string
}

// ChatroomInfo represents information about a chatroom for the API
type ChatroomInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	UserCount   int    `json:"userCount"`
	CreatedAt   string `json:"createdAt"`
	CreatorID   string `json:"creatorId"` // Added field to track who created the room
}

// ChatRoom manages the connected clients and message broadcasting
type ChatRoom struct {
	ID          string
	Name        string
	Description string
	CreatorID   string // Added field to track who created the room
	clients     map[string]*Client
	register    chan *Client
	unregister  chan *Client
	broadcast   chan Message
	messages    []Message // Store for chat history
	maxHistory  int       // Maximum number of messages to store
	mu          sync.Mutex
	createdAt   time.Time
}

// ChatServer manages multiple chat rooms
type ChatServer struct {
	rooms         map[string]*ChatRoom
	users         map[string]*User  // Map of userID to User
	sessions      map[string]string // Map of session token to userID
	roomsMutex    sync.RWMutex
	usersMutex    sync.RWMutex
	sessionsMutex sync.RWMutex
}

// NewChatServer creates a new chat server instance
func NewChatServer() *ChatServer {
	return &ChatServer{
		rooms:    make(map[string]*ChatRoom),
		users:    make(map[string]*User),
		sessions: make(map[string]string),
	}
}

// NewChatRoom creates a new chat room instance
func NewChatRoom(id, name, description, creatorID string) *ChatRoom {
	fmt.Printf("Creating new chat room: %s (%s)\n", name, id)
	return &ChatRoom{
		ID:          id,
		Name:        name,
		Description: description,
		CreatorID:   creatorID,
		clients:     make(map[string]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan Message),
		messages:    make([]Message, 0, 100),
		maxHistory:  100, // Store last 100 messages by default
		createdAt:   time.Now(),
	}
}

// GetChatRoomInfo returns information about a chatroom
func (cr *ChatRoom) GetInfo() ChatroomInfo {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	return ChatroomInfo{
		ID:          cr.ID,
		Name:        cr.Name,
		Description: cr.Description,
		UserCount:   len(cr.clients),
		CreatedAt:   cr.createdAt.Format(time.RFC3339),
		CreatorID:   cr.CreatorID,
	}
}

// GetRoom returns a chat room by ID, creating it if it doesn't exist
func (s *ChatServer) GetRoom(id string) *ChatRoom {
	s.roomsMutex.RLock()
	room, exists := s.rooms[id]
	s.roomsMutex.RUnlock()

	if exists {
		return room
	}

	return nil
}

// CreateRoom creates a new chat room
func (s *ChatServer) CreateRoom(name, description, creatorID string) *ChatRoom {
	id := uuid.New().String()
	room := NewChatRoom(id, name, description, creatorID)

	s.roomsMutex.Lock()
	s.rooms[id] = room
	s.roomsMutex.Unlock()

	// Start the room's event loop
	go room.Run()

	return room
}

// ListRooms returns information about all chat rooms
func (s *ChatServer) ListRooms() []ChatroomInfo {
	s.roomsMutex.RLock()
	defer s.roomsMutex.RUnlock()

	rooms := make([]ChatroomInfo, 0, len(s.rooms))
	for _, room := range s.rooms {
		rooms = append(rooms, room.GetInfo())
	}

	return rooms
}

// ListUserRooms returns rooms created by a specific user
func (s *ChatServer) ListUserRooms(userID string) []ChatroomInfo {
	s.roomsMutex.RLock()
	defer s.roomsMutex.RUnlock()

	rooms := make([]ChatroomInfo, 0)
	for _, room := range s.rooms {
		if room.CreatorID == userID {
			rooms = append(rooms, room.GetInfo())
		}
	}

	return rooms
}

// RegisterUser creates a new user account
func (s *ChatServer) RegisterUser(username, password string) (*User, error) {
	s.usersMutex.Lock()
	defer s.usersMutex.Unlock()

	// Check if username already exists
	for _, user := range s.users {
		if user.Username == username {
			return nil, fmt.Errorf("username already exists")
		}
	}

	userID := uuid.New().String()
	user := &User{
		ID:       userID,
		Username: username,
		Password: password, // In a real app, this would be hashed
	}

	s.users[userID] = user
	return user, nil
}

// AuthenticateUser checks credentials and returns a session token
func (s *ChatServer) AuthenticateUser(username, password string) (string, string, error) {
	s.usersMutex.RLock()
	defer s.usersMutex.RUnlock()

	for _, user := range s.users {
		if user.Username == username && user.Password == password {
			sessionToken := uuid.New().String()

			s.sessionsMutex.Lock()
			s.sessions[sessionToken] = user.ID
			s.sessionsMutex.Unlock()

			return sessionToken, user.ID, nil
		}
	}

	return "", "", fmt.Errorf("invalid credentials")
}

// ValidateSession checks if a session token is valid and returns the userID
func (s *ChatServer) ValidateSession(token string) (string, error) {
	s.sessionsMutex.RLock()
	defer s.sessionsMutex.RUnlock()

	userID, exists := s.sessions[token]
	if !exists {
		return "", fmt.Errorf("invalid session")
	}

	return userID, nil
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all connections
		return true
	},
}

// Run starts the chatroom event loop
func (cr *ChatRoom) Run() {
	for {
		select {
		case client := <-cr.register:
			cr.mu.Lock()
			cr.clients[client.ID] = client
			cr.mu.Unlock()

			// Notify everyone about the new user
			cr.broadcast <- Message{
				Type:      "userJoined",
				Content:   client.Name + " has joined the chat",
				Sender:    "system",
				Timestamp: time.Now(),
			}

			// Send the list of online users to the new client
			cr.sendUserList(client)

			// Message history is disabled to keep chat privacy
			// No history is sent to newly joined users

		case client := <-cr.unregister:
			cr.mu.Lock()
			if _, ok := cr.clients[client.ID]; ok {
				delete(cr.clients, client.ID)
				close := Message{
					Type:      "userLeft",
					Content:   client.Name + " has left the chat",
					Sender:    "system",
					Timestamp: time.Now(),
				}
				cr.mu.Unlock()
				cr.broadcast <- close
			} else {
				cr.mu.Unlock()
			}

		case message := <-cr.broadcast:
			cr.mu.Lock()
			// Store the message in history
			cr.messages = append(cr.messages, message)
			if len(cr.messages) > cr.maxHistory {
				cr.messages = cr.messages[1:]
			}

			for _, client := range cr.clients {
				err := client.Conn.WriteJSON(message)
				if err != nil {
					fmt.Println("Error broadcasting message:", err)
					client.Conn.Close()
					delete(cr.clients, client.ID)
				}
			}
			cr.mu.Unlock()
		}
	}
}

// sendUserList sends the current list of online users to a client
func (cr *ChatRoom) sendUserList(client *Client) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	userList := make([]string, 0, len(cr.clients))
	for _, c := range cr.clients {
		userList = append(userList, c.Name)
	}

	userListMsg := Message{
		Type:      "userList",
		Content:   "",
		Sender:    "system",
		Timestamp: time.Now(),
	}

	// Marshal the user list to JSON and set as content
	userListJSON, err := json.Marshal(userList)
	if err == nil {
		userListMsg.Content = string(userListJSON)
	}

	client.Conn.WriteJSON(userListMsg)
}

// clientCounter helps generate unique IDs for clients
var clientCounter = 0

func wsHandler(w http.ResponseWriter, r *http.Request, server *ChatServer) {
	// Get room ID from the URL query
	roomID := r.URL.Query().Get("roomId")
	if roomID == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	room := server.GetRoom(roomID)
	if room == nil {
		http.Error(w, "Room not found", http.StatusNotFound)
		return
	}

	// Update the HTTP connection to a WebSocket connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error during connection upgrade:", err)
		return
	}

	clientCounter++
	clientID := fmt.Sprintf("user_%d", clientCounter)

	// Wait for the client to send their username first
	var initMsg Message
	err = conn.ReadJSON(&initMsg)
	if err != nil {
		fmt.Println("Error reading initial message:", err)
		conn.Close()
		return
	}

	client := &Client{
		ID:   clientID,
		Conn: conn,
		Name: initMsg.Sender,
	}

	room.register <- client

	fmt.Printf("Client connected to room %s: %s (%s)\n", room.Name, client.Name, client.ID)
	handleConnection(conn, client, room)
}

// handleConnection manages a WebSocket connection for a client
func handleConnection(conn *websocket.Conn, client *Client, cr *ChatRoom) {
	defer func() {
		cr.unregister <- client
		conn.Close()
		fmt.Printf("Client disconnected from room %s: %s (%s)\n", cr.Name, client.Name, client.ID)
	}()

	for {
		var message Message
		err := conn.ReadJSON(&message)
		if err != nil {
			fmt.Println("Error reading message:", err)
			break
		}

		fmt.Printf("Received message from %s: %s (Type: %s)\n", client.Name, message.Content, message.Type)

		// Handle different message types
		if message.Type == "refreshUserList" {
			// Send updated user list to the requesting client
			cr.sendUserList(client)
			continue
		}

		// Set the correct sender name and timestamp
		message.Sender = client.Name
		message.Timestamp = time.Now()

		// Broadcast the message to all clients
		cr.broadcast <- message
	}
}

func main() {
	server := NewChatServer()

	// Create some default rooms
	server.CreateRoom("General Chat", "A general chat room for everyone", "system")
	server.CreateRoom("Tech Talk", "Discuss technology and programming", "system")
	server.CreateRoom("Random", "Chat about anything and everything", "system")

	// Create some test users
	server.RegisterUser("alice", "password123")
	server.RegisterUser("bob", "password123")

	// Enable CORS middleware
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	// Create router
	mux := http.NewServeMux()

	// API endpoint for user registration
	mux.HandleFunc("/api/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var userData struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&userData); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		user, err := server.RegisterUser(userData.Username, userData.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "User registered successfully",
			"userId":  user.ID,
		})
	})

	// API endpoint for user login
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var loginData struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}

		if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		sessionToken, userID, err := server.AuthenticateUser(loginData.Username, loginData.Password)
		if err != nil {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"message":  "Login successful",
			"token":    sessionToken,
			"userId":   userID,
			"username": loginData.Username,
		})
	})

	// API endpoint for creating a new chatroom
	mux.HandleFunc("/api/chatrooms/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Get token from Authorization header
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		// Validate session
		userID, err := server.ValidateSession(token)
		if err != nil {
			http.Error(w, "Invalid session", http.StatusUnauthorized)
			return
		}

		var roomData struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}

		if err := json.NewDecoder(r.Body).Decode(&roomData); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		room := server.CreateRoom(roomData.Name, roomData.Description, userID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(room.GetInfo())
	})

	// API endpoint to list all chat rooms
	mux.HandleFunc("/api/chatrooms", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rooms := server.ListRooms()
		json.NewEncoder(w).Encode(rooms)
	})

	// API endpoint to list user's created rooms
	mux.HandleFunc("/api/chatrooms/my", func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		// Validate session
		userID, err := server.ValidateSession(token)
		if err != nil {
			http.Error(w, "Invalid session", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		rooms := server.ListUserRooms(userID)
		json.NewEncoder(w).Encode(rooms)
	})

	// WebSocket endpoint for chat connections
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsHandler(w, r, server)
	})

	// Apply CORS middleware
	handler := corsMiddleware(mux)

	fmt.Println("Chatroom Server started on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("- WebSocket: ws://localhost:8080/ws?roomId=<room-id>")
	fmt.Println("- Registration: POST http://localhost:8080/api/register")
	fmt.Println("- Login: POST http://localhost:8080/api/login")
	fmt.Println("- Chatrooms API: http://localhost:8080/api/chatrooms")
	fmt.Println("- User's Chatrooms API: http://localhost:8080/api/chatrooms/my")
	fmt.Println("- Create Chatroom API: POST http://localhost:8080/api/chatrooms/create")

	err := http.ListenAndServe(":8080", handler)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
}
