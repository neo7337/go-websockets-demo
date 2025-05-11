package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client
var ctx = context.Background()

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Hub struct {
	clients    map[*websocket.Conn]string
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]string),
		broadcast:  make(chan []byte),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			// on register, set a unique session in Redis
			userID := generateUserID()
			h.clients[conn] = userID
			err := rdb.Set(ctx, userID, conn.RemoteAddr().String(), 0).Err()
			if err != nil {
				fmt.Println("Error setting user ID in Redis:", err)
				continue
			}
			fmt.Println("Client connected:", conn.RemoteAddr())
		case conn := <-h.unregister:
			userId, ok := h.clients[conn]
			if ok {
				// on unregister, delete the session from Redis
				err := rdb.Del(ctx, userId).Err()
				if err != nil {
					fmt.Println("Error deleting user ID from Redis:", err)
				}
				// remove the client from the hub
				delete(h.clients, conn)
				conn.Close()
				fmt.Println("Client disconnected:", conn.RemoteAddr())
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				err := client.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					fmt.Println("Error writing message to client:", err)
					client.Close()
					delete(h.clients, client)
				} else {
					fmt.Println("Message sent to client:", client.RemoteAddr())
				}
			}
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Could not upgrade connection", http.StatusInternalServerError)
		return
	}
	hub.register <- conn

	go func() {
		defer func() {
			hub.unregister <- conn
			conn.Close()
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				fmt.Println("Error reading message:", err)
				break
			}
			hub.broadcast <- message
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

	hub := newHub()

	go hub.Run()

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})

	mux.HandleFunc("/register", registerUserHandler)

	handler := corsMiddleware(mux)

	port := ":8080"
	println("Server started on port", port)
	if err := http.ListenAndServe(port, handler); err != nil {
		println("Error starting server:", err)
	}
}
