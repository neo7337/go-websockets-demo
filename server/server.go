package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// Chatroom struct defines the properties of a chatroom
type Chatroom struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatorID   string    `json:"creatorId"`
	CreatedAt   time.Time `json:"createdAt"`
	UserCount   int       `json:"userCount"`
}

var rdb *redis.Client
var ctx = context.Background()

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Hub manages WebSocket connections for a specific chatroom
type Hub struct {
	clients    map[*websocket.Conn]string
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	roomID     string
}

// Map to keep track of all active hubs (one per chatroom)
var chatHubs = make(map[string]*Hub)
var hubsMutex = &sync.Mutex{}

func newHub(roomID string) *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]string),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		roomID:     roomID,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			// on register, set a unique session in Redis
			userID := generateUserID()
			h.clients[conn] = userID

			// Increment user count in chatroom
			h.updateUserCount(1)

			fmt.Printf("Client connected to room %s: %s\n", h.roomID, conn.RemoteAddr())

			// Notify all clients in the room about new user
			h.sendSystemMessage("A new user has joined the chat")

		case conn := <-h.unregister:
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()

				// Decrement user count in chatroom
				h.updateUserCount(-1)

				fmt.Printf("Client disconnected from room %s: %s\n", h.roomID, conn.RemoteAddr())

				// Notify all clients in the room
				h.sendSystemMessage("A user has left the chat")

				// If no clients left, consider cleaning up the hub
				if len(h.clients) == 0 {
					// Keep the hub for now, as users might rejoin
					// In a production system you might want to implement a cleanup mechanism
				}
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					fmt.Println("Error writing message to client:", err)
					client.Close()
					delete(h.clients, client)
				}
			}
		}
	}
}

// updateUserCount updates the user count in the chatroom stored in Redis
func (h *Hub) updateUserCount(delta int) {
	chatroomKey := "chatroom:" + h.roomID

	// Get current chatroom data
	chatroomJSON, err := rdb.Get(ctx, chatroomKey).Result()
	if err != nil {
		fmt.Printf("Error getting chatroom data: %v\n", err)
		return
	}

	var chatroom Chatroom
	if err := json.Unmarshal([]byte(chatroomJSON), &chatroom); err != nil {
		fmt.Printf("Error unmarshalling chatroom data: %v\n", err)
		return
	}

	// Update user count
	chatroom.UserCount = len(h.clients) // Use actual count instead of incrementing

	// Save updated chatroom
	updatedJSON, err := json.Marshal(chatroom)
	if err != nil {
		fmt.Printf("Error marshalling chatroom data: %v\n", err)
		return
	}

	if err := rdb.Set(ctx, chatroomKey, updatedJSON, 0).Err(); err != nil {
		fmt.Printf("Error updating chatroom data: %v\n", err)
	}
}

// sendSystemMessage broadcasts a system message to all clients in the room
func (h *Hub) sendSystemMessage(text string) {
	msg := map[string]interface{}{
		"type":      "system",
		"content":   text,
		"sender":    "system",
		"timestamp": time.Now(),
	}

	msgJSON, err := json.Marshal(msg)
	if err == nil {
		h.broadcast <- msgJSON
	}
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	// Get room ID from query parameters
	roomID := r.URL.Query().Get("roomId")
	if roomID == "" {
		http.Error(w, "Room ID is required", http.StatusBadRequest)
		return
	}

	// Check if the chatroom exists in Redis
	existsVal, err := rdb.Exists(ctx, "chatroom:"+roomID).Result()
	if err != nil || existsVal == 0 {
		http.Error(w, "Chatroom not found", http.StatusNotFound)
		return
	}

	// Get or create hub for this room
	hubsMutex.Lock()
	hub, ok := chatHubs[roomID]
	if !ok {
		hub = newHub(roomID)
		chatHubs[roomID] = hub
		go hub.Run()
	}
	hubsMutex.Unlock()

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Could not upgrade connection", http.StatusInternalServerError)
		return
	}

	// Register the client with the hub
	hub.register <- conn

	// Handle incoming messages
	go func() {
		defer func() {
			hub.unregister <- conn
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				fmt.Println("Error reading message:", err)
				break
			}

			// Process and broadcast the message
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				// Add timestamp if not present
				if _, ok := msg["timestamp"]; !ok {
					msg["timestamp"] = time.Now()
				}

				// Re-marshal with added/modified fields
				updatedMsg, err := json.Marshal(msg)
				if err == nil {
					hub.broadcast <- updatedMsg
				} else {
					hub.broadcast <- message
				}
			} else {
				// If we couldn't parse as JSON, broadcast as-is
				hub.broadcast <- message
			}
		}
	}()
}

func registerUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	type User struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// check if the username already exists
	exists, err := rdb.Exists(ctx, user.Username).Result()
	if err != nil {
		http.Error(w, "Error checking username", http.StatusInternalServerError)
		return
	}

	if exists > 0 {
		http.Error(w, "Username already exists", http.StatusConflict)
		return
	}

	// Store username and hashed password in Redis
	hasedPassword := hashPassword(user.Password)
	err = rdb.Set(ctx, user.Username, hasedPassword, 0).Err()
	if err != nil {
		http.Error(w, "Error storing user data", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "User registered successfully",
		"username": user.Username,
	})
}

func loginUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	type LoginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	var loginReq LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// check if the username exists in Redis
	storedHash, err := rdb.Get(ctx, loginReq.Username).Result()
	if err != nil {
		if err == redis.Nil {
			http.Error(w, "Username not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error checking username", http.StatusInternalServerError)
		}
		return
	}

	if hashPassword(loginReq.Password) != storedHash {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	// Generate a session token (you can use JWT or any other method)
	sessionToken := generateSessionToken(loginReq.Username)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"message":       "Login successful",
		"session_token": sessionToken,
		"username":      loginReq.Username,
	})
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	// Check if the user is logged in
	sessionToken := r.Header.Get("Authorization")
	if sessionToken == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	// In a real application, you would validate the session token here
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Pong",
	})
}

func createChatroomHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Get token from Authorization header
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Authorization required", http.StatusUnauthorized)
		return
	}

	// In a real application with proper session management, you would validate the token
	// and get the user ID. For this implementation, we'll extract username from token
	username := extractUsernameFromToken(token)
	if username == "" {
		http.Error(w, "Invalid session token", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var chatroomRequest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&chatroomRequest); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if chatroomRequest.Name == "" {
		http.Error(w, "Chatroom name is required", http.StatusBadRequest)
		return
	}

	// Create a unique ID for the chatroom
	chatroomID := generateChatroomID()

	// Create chatroom object
	chatroom := Chatroom{
		ID:          chatroomID,
		Name:        chatroomRequest.Name,
		Description: chatroomRequest.Description,
		CreatorID:   username,
		CreatedAt:   time.Now(),
		UserCount:   0,
	}

	// Serialize to JSON for Redis storage
	chatroomJSON, err := json.Marshal(chatroom)
	if err != nil {
		http.Error(w, "Error creating chatroom", http.StatusInternalServerError)
		return
	}

	// Store in Redis - individual chatroom
	err = rdb.Set(ctx, "chatroom:"+chatroomID, chatroomJSON, 0).Err()
	if err != nil {
		http.Error(w, "Error storing chatroom data", http.StatusInternalServerError)
		return
	}

	// Add to chatrooms index
	err = rdb.SAdd(ctx, "chatrooms", chatroomID).Err()
	if err != nil {
		http.Error(w, "Error updating chatroom index", http.StatusInternalServerError)
		return
	}

	// Add to user's chatrooms
	err = rdb.SAdd(ctx, "user:"+username+":chatrooms", chatroomID).Err()
	if err != nil {
		http.Error(w, "Error updating user's chatrooms", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(chatroom)
}

func chatroomsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Get all chatroom IDs from the index
	chatroomIDs, err := rdb.SMembers(ctx, "chatrooms").Result()
	if err != nil {
		http.Error(w, "Error fetching chatrooms", http.StatusInternalServerError)
		return
	}

	chatrooms := []Chatroom{}

	// For each ID, get the chatroom data
	for _, id := range chatroomIDs {
		chatroomJSON, err := rdb.Get(ctx, "chatroom:"+id).Result()
		if err != nil {
			continue // Skip this chatroom if there was an error
		}

		var chatroom Chatroom
		if err := json.Unmarshal([]byte(chatroomJSON), &chatroom); err != nil {
			continue // Skip this chatroom if there was an error
		}

		chatrooms = append(chatrooms, chatroom)
	}

	// Return the list of chatrooms
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"chatrooms": chatrooms,
	})
}

func userChatroomsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Get token from Authorization header
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Authorization required", http.StatusUnauthorized)
		return
	}

	// Extract username from token
	username := extractUsernameFromToken(token)
	if username == "" {
		http.Error(w, "Invalid session token", http.StatusUnauthorized)
		return
	}

	// Get user's chatroom IDs from Redis
	userChatroomIDs, err := rdb.SMembers(ctx, "user:"+username+":chatrooms").Result()
	if err != nil {
		http.Error(w, "Error fetching user's chatrooms", http.StatusInternalServerError)
		return
	}

	chatrooms := []Chatroom{}

	// For each ID, get the chatroom data
	for _, id := range userChatroomIDs {
		chatroomJSON, err := rdb.Get(ctx, "chatroom:"+id).Result()
		if err != nil {
			continue // Skip this chatroom if there was an error
		}

		var chatroom Chatroom
		if err := json.Unmarshal([]byte(chatroomJSON), &chatroom); err != nil {
			continue // Skip this chatroom if there was an error
		}

		chatrooms = append(chatrooms, chatroom)
	}

	// Return the list of user's chatrooms
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatrooms)
}

func extractUsernameFromToken(token string) string {
	// In a real application with proper token validation,
	// you would decode and validate JWT or other token format.
	// This is a simple implementation for this demo.
	if token == "" {
		return ""
	}

	// Split by underscore, assuming token format is "username_timestamp"
	parts := strings.Split(token, "_")
	if len(parts) < 1 {
		return ""
	}

	return parts[0]
}

func generateChatroomID() string {
	return fmt.Sprintf("chatroom_%d", time.Now().UnixNano())
}

func generateSessionToken(username string) string {
	// In a real application, you would use a more secure method to generate session tokens
	return fmt.Sprintf("%s_%d", username, time.Now().Unix())
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:1])
}

// Generate a unique user ID (you can use a better strategy in production)
func generateUserID() string {
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}

func main() {

	// initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0, // use default DB
	})

	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		fmt.Println("Error connecting to Redis:", err)
		return
	}
	fmt.Println("Connected to Redis")

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

	mux := http.NewServeMux()

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(w, r)
	})

	mux.HandleFunc("/register", registerUserHandler)
	mux.HandleFunc("/login", loginUserHandler)
	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/api/chatrooms", chatroomsHandler)
	mux.HandleFunc("/api/chatrooms/create", createChatroomHandler)
	mux.HandleFunc("/api/chatrooms/my", userChatroomsHandler)

	handler := corsMiddleware(mux)

	port := ":8080"
	fmt.Println("Chatroom Server started on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("- WebSocket: ws://localhost:8080/ws?roomId=<room-id>")
	fmt.Println("- Registration: POST http://localhost:8080/register")
	fmt.Println("- Login: POST http://localhost:8080/login")
	fmt.Println("- Chatrooms API: http://localhost:8080/api/chatrooms")
	fmt.Println("- User's Chatrooms API: http://localhost:8080/api/chatrooms/my")
	fmt.Println("- Create Chatroom API: POST http://localhost:8080/api/chatrooms/create")

	if err := http.ListenAndServe(port, handler); err != nil {
		fmt.Println("Error starting server:", err)
	}
}
