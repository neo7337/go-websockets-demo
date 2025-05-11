import { useState, useEffect, useRef } from 'react';
import './App.css';

// ChatroomCard component to display a single chatroom
function ChatroomCard({ room, onSelect, isSelected }) {
  return (
    <div 
      className={`chatroom-card ${isSelected ? 'selected' : ''}`}
      onClick={() => onSelect(room)}
    >
      <h3>{room.name}</h3>
      <p>{room.description}</p>
      <div className="chatroom-info">
        <span>{room.userCount} users online</span>
      </div>
    </div>
  );
}

// CreateRoomForm component for creating a new chatroom
function CreateRoomForm({ onRoomCreated }) {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [isCreating, setIsCreating] = useState(false);
  
  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!name.trim()) return;
    
    setIsCreating(true);
    try {
      const token = localStorage.getItem('chatToken');
      const response = await fetch('http://localhost:8080/api/chatrooms/create', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': token
        },
        body: JSON.stringify({ name, description })
      });
      
      if (!response.ok) {
        throw new Error('Failed to create chatroom');
      }
      
      const newRoom = await response.json();
      onRoomCreated(newRoom);
      setName('');
      setDescription('');
    } catch (error) {
      console.error('Error creating chatroom:', error);
      alert('Failed to create chatroom. Please try again.');
    } finally {
      setIsCreating(false);
    }
  };
  
  return (
    <div className="create-room-form">
      <h3>Create New Chatroom</h3>
      <form onSubmit={handleSubmit}>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Room name"
          required
        />
        <textarea
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Room description"
        />
        <button type="submit" disabled={isCreating}>
          {isCreating ? 'Creating...' : 'Create Room'}
        </button>
      </form>
    </div>
  );
}

function App() {
  // Authentication states
  const [isLoggedIn, setIsLoggedIn] = useState(false);
  const [isRegistering, setIsRegistering] = useState(false);
  const [authUsername, setAuthUsername] = useState('');
  const [authPassword, setAuthPassword] = useState('');
  const [userId, setUserId] = useState('');
  const [loginError, setLoginError] = useState('');
  
  // App states
  const [connected, setConnected] = useState(false);
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState('');
  const [username, setUsername] = useState('');
  const [onlineUsers, setOnlineUsers] = useState([]);
  const [isJoined, setIsJoined] = useState(false);
  const [allChatrooms, setAllChatrooms] = useState([]);
  const [userChatrooms, setUserChatrooms] = useState([]);
  const [selectedRoom, setSelectedRoom] = useState(null);
  const [loadingRooms, setLoadingRooms] = useState(false);
  const [showCreateRoom, setShowCreateRoom] = useState(false);
  const socketRef = useRef(null);
  const messagesEndRef = useRef(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  };

  // Check if user is already logged in
  useEffect(() => {
    const token = localStorage.getItem('chatToken');
    const storedUsername = localStorage.getItem('chatUsername');
    const storedUserId = localStorage.getItem('chatUserId');
    
    if (token && storedUsername && storedUserId) {
      setIsLoggedIn(true);
      setUsername(storedUsername);
      setUserId(storedUserId);
    }
  }, []);

  // Fetch chatrooms when component mounts or user logs in
  useEffect(() => {
    if (isLoggedIn) {
      fetchChatrooms();
      fetchUserChatrooms();
    }
  }, [isLoggedIn]);

  // Fetch all available chatrooms from the server
  const fetchChatrooms = async () => {
    setLoadingRooms(true);
    try {
      const response = await fetch('http://localhost:8080/api/chatrooms');
      if (!response.ok) {
        throw new Error('Failed to fetch chatrooms');
      }
      const data = await response.json();
      setAllChatrooms(data);
    } catch (error) {
      console.error('Error fetching chatrooms:', error);
      setMessages(prev => [...prev, {
        type: 'system', 
        text: 'Error loading chatrooms. Please try again later.',
        sender: 'system'
      }]);
    } finally {
      setLoadingRooms(false);
    }
  };

  // Fetch user's created chatrooms
  const fetchUserChatrooms = async () => {
    try {
      const token = localStorage.getItem('chatToken');
      if (!token) return;
      
      const response = await fetch('http://localhost:8080/api/chatrooms/my', {
        headers: {
          'Authorization': token
        }
      });
      
      if (!response.ok) {
        throw new Error('Failed to fetch user chatrooms');
      }
      
      const data = await response.json();
      setUserChatrooms(data);
    } catch (error) {
      console.error('Error fetching user chatrooms:', error);
    }
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  // Debug effect to log when onlineUsers changes
  useEffect(() => {
    console.log('Online users updated:', onlineUsers);
  }, [onlineUsers]);

  // Handle user registration
  const handleRegister = async (e) => {
    e.preventDefault();
    setLoginError('');
    
    try {
      const response = await fetch('http://localhost:8080/api/register', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          username: authUsername,
          password: authPassword
        })
      });
      
      const data = await response.json();
      
      if (!response.ok) {
        throw new Error(data.message || 'Registration failed');
      }
      
      // After successful registration, switch to login form
      setIsRegistering(false);
      alert('Registration successful! You can now login.');
    } catch (error) {
      console.error('Registration error:', error);
      setLoginError(error.message || 'Registration failed. Please try again.');
    }
  };

  // Handle user login
  const handleLogin = async (e) => {
    e.preventDefault();
    setLoginError('');
    
    try {
      const response = await fetch('http://localhost:8080/api/login', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          username: authUsername,
          password: authPassword
        })
      });
      
      const data = await response.json();
      
      if (!response.ok) {
        throw new Error(data.message || 'Login failed');
      }
      
      // Save auth data
      localStorage.setItem('chatToken', data.token);
      localStorage.setItem('chatUsername', data.username);
      localStorage.setItem('chatUserId', data.userId);
      
      setIsLoggedIn(true);
      setUsername(data.username);
      setUserId(data.userId);
      setAuthUsername('');
      setAuthPassword('');
    } catch (error) {
      console.error('Login error:', error);
      setLoginError(error.message || 'Invalid username or password');
    }
  };

  // Handle logout
  const handleLogout = () => {
    localStorage.removeItem('chatToken');
    localStorage.removeItem('chatUsername');
    localStorage.removeItem('chatUserId');
    
    setIsLoggedIn(false);
    setIsJoined(false);
    setUsername('');
    setUserId('');
    setSelectedRoom(null);
    setMessages([]);
    
    if (socketRef.current) {
      socketRef.current.close();
    }
  };

  // Handle room creation callback
  const handleRoomCreated = (newRoom) => {
    setUserChatrooms(prev => [newRoom, ...prev]);
    setAllChatrooms(prev => [newRoom, ...prev]);
    setShowCreateRoom(false);
  };

  const connectWebSocket = () => {
    if (!username) {
      alert('Please enter a username');
      return;
    }
    
    if (!selectedRoom) {
      alert('Please select a chatroom');
      return;
    }

    const ws = new WebSocket(`ws://localhost:8080/ws?roomId=${selectedRoom.id}`);
    
    ws.onopen = () => {
      console.log('Connected to WebSocket server');
      setConnected(true);
      
      // Send username as the first message
      const initMessage = {
        type: 'init',
        content: 'Joining chat',
        sender: username
      };
      
      ws.send(JSON.stringify(initMessage));
      setIsJoined(true);
    };
    
    ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      console.log('Message from server:', message);
      
      switch (message.type) {
        case 'userList':
          try {
            const userList = JSON.parse(message.content);
            console.log('User list received:', userList);
            setOnlineUsers(userList);
          } catch (error) {
            console.error('Error parsing user list:', error);
          }
          break;
          
        case 'userJoined':
        case 'userLeft':
          setMessages(prev => [...prev, {
            type: 'system',
            text: message.content,
            sender: 'system'
          }]);
          
          // Request an updated user list after users join/leave
          const refreshRequest = {
            type: 'refreshUserList',
            content: '',
            sender: username
          };
          ws.send(JSON.stringify(refreshRequest));
          break;
          
        default:
          setMessages(prev => [...prev, {
            type: 'chat',
            text: message.content,
            sender: message.sender
          }]);
      }
    };
    
    ws.onclose = () => {
      console.log('Disconnected from WebSocket server');
      setConnected(false);
      setIsJoined(false);
      setMessages(prev => [...prev, { 
        type: 'system',
        text: 'Disconnected from server',
        sender: 'system'
      }]);
      
      // Try to reconnect after 3 seconds
      setTimeout(() => {
        if (socketRef.current === ws) {
          connectWebSocket();
        }
      }, 3000);
    };
    
    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      setMessages(prev => [...prev, {
        type: 'system',
        text: 'Error connecting to server',
        sender: 'system'
      }]);
    };
    
    socketRef.current = ws;
  };
  
  const sendMessage = (e) => {
    e.preventDefault();
    if (!inputMessage.trim() || !socketRef.current || socketRef.current.readyState !== WebSocket.OPEN) return;
    
    const message = {
      type: 'chat',
      content: inputMessage,
      sender: username
    };
    
    socketRef.current.send(JSON.stringify(message));
    setInputMessage('');
  };

  const handleRoomSelect = (room) => {
    setSelectedRoom(room);
  };

  const joinChat = (e) => {
    e.preventDefault();
    if (!username) {
      alert('Please login first');
      return;
    }
    
    if (!selectedRoom) {
      alert('Please select a chatroom');
      return;
    }
    
    connectWebSocket();
  };

  const leaveChatroom = () => {
    if (socketRef.current) {
      socketRef.current.close();
    }
    setMessages([]);
    setIsJoined(false);
    setConnected(false);
    setSelectedRoom(null);
  };

  // If not logged in, show login/register form
  if (!isLoggedIn) {
    return (
      <div className="App">
        <header className="App-header">
          <h1>WebSocket Chatroom</h1>
        </header>
        
        <div className="auth-container">
          <h2>{isRegistering ? 'Create Account' : 'Login'}</h2>
          
          {loginError && <div className="error-message">{loginError}</div>}
          
          <form onSubmit={isRegistering ? handleRegister : handleLogin} className="auth-form">
            <input
              type="text"
              value={authUsername}
              onChange={(e) => setAuthUsername(e.target.value)}
              placeholder="Username"
              required
            />
            <input
              type="password"
              value={authPassword}
              onChange={(e) => setAuthPassword(e.target.value)}
              placeholder="Password"
              required
            />
            <button type="submit">{isRegistering ? 'Register' : 'Login'}</button>
          </form>
          
          <div className="auth-switch">
            {isRegistering ? (
              <p>Already have an account? <button onClick={() => setIsRegistering(false)}>Login</button></p>
            ) : (
              <p>Don't have an account? <button onClick={() => setIsRegistering(true)}>Register</button></p>
            )}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="App">
      <header className="App-header">
        <h1>WebSocket Chatroom</h1>
        <div className="user-section">
          <span>Logged in as: {username}</span>
          <button className="logout-btn" onClick={handleLogout}>Logout</button>
        </div>
        <div className="connection-status">
          Status: {connected ? '🟢 Connected' : '🔴 Disconnected'}
        </div>
      </header>
      
      {!isJoined ? (
        <div className="join-container">
          <button 
            className="create-room-toggle" 
            onClick={() => setShowCreateRoom(!showCreateRoom)}
          >
            {showCreateRoom ? 'Hide Room Creation' : 'Create New Room'}
          </button>
          
          {showCreateRoom && (
            <CreateRoomForm onRoomCreated={handleRoomCreated} />
          )}
          
          <h2>Join a Chatroom</h2>
          
          {userChatrooms.length > 0 && (
            <div className="my-chatrooms-section">
              <h3>My Chatrooms</h3>
              <div className="chatroom-list">
                {userChatrooms.map(room => (
                  <ChatroomCard 
                    key={room.id}
                    room={room}
                    onSelect={handleRoomSelect}
                    isSelected={selectedRoom && selectedRoom.id === room.id}
                  />
                ))}
              </div>
            </div>
          )}
          
          <h3>All Chatrooms</h3>
          <div className="chatroom-selector">
            {loadingRooms ? (
              <div className="loading">Loading available chatrooms...</div>
            ) : allChatrooms.length === 0 ? (
              <div className="no-rooms">
                <p>No chatrooms available</p>
                <button onClick={fetchChatrooms}>Refresh</button>
              </div>
            ) : (
              <div className="chatroom-list">
                {allChatrooms.map(room => (
                  <ChatroomCard 
                    key={room.id}
                    room={room}
                    onSelect={handleRoomSelect}
                    isSelected={selectedRoom && selectedRoom.id === room.id}
                  />
                ))}
              </div>
            )}
          </div>
          
          {selectedRoom && (
            <div className="selected-room-info">
              <h3>Selected: {selectedRoom.name}</h3>
              <p>{selectedRoom.description}</p>
            </div>
          )}
          
          <button 
            className="join-btn" 
            onClick={joinChat} 
            disabled={!selectedRoom}
          >
            Join Chat
          </button>
        </div>
      ) : (
        <main className="chat-container">
          <div className="chat-sidebar">
            <div className="room-header">
              <h3>{selectedRoom?.name}</h3>
              <p>{selectedRoom?.description}</p>
              <button className="leave-btn" onClick={leaveChatroom}>Leave Room</button>
            </div>
            <h3>Online Users ({onlineUsers.length})</h3>
            <ul className="user-list">
              {onlineUsers.map((user, index) => (
                <li key={index} className="user-item">
                  {user} {user === username && ' (you)'}
                </li>
              ))}
            </ul>
          </div>
          
          <div className="chat-main">
            <div className="message-list">
              {messages.map((msg, index) => (
                <div 
                  key={index} 
                  className={`message ${msg.type} ${msg.sender === username ? 'own' : ''}`}
                >
                  {msg.type !== 'system' && (
                    <span className="sender">{msg.sender}</span>
                  )}
                  <span className="text">{msg.text}</span>
                </div>
              ))}
              <div ref={messagesEndRef} />
            </div>
            
            <form onSubmit={sendMessage} className="message-form">
              <input
                type="text"
                value={inputMessage}
                onChange={(e) => setInputMessage(e.target.value)}
                placeholder="Type a message..."
                disabled={!connected}
              />
              <button type="submit" disabled={!connected}>Send</button>
            </form>
          </div>
        </main>
      )}
    </div>
  );
}

export default App;
