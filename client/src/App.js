import { useState, useEffect, useRef } from 'react';
import './App.css';

function App() {
  // App states
  const [connected, setConnected] = useState(false);
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState('');
  const [username, setUsername] = useState('');
  const [onlineUsers, setOnlineUsers] = useState([]);
  const [isJoined, setIsJoined] = useState(false);
  const socketRef = useRef(null);
  const messagesEndRef = useRef(null);

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  // Debug effect to log when onlineUsers changes
  useEffect(() => {
    console.log('Online users updated:', onlineUsers);
  }, [onlineUsers]);

  const connectWebSocket = () => {
    if (!username) {
      alert('Please enter a username');
      return;
    }
    
    const ws = new WebSocket('ws://localhost:8080/ws?roomId=default_room');
    
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
    console.log(message)
    console.log(socketRef.current)
    socketRef.current.send(JSON.stringify(message));
    setInputMessage('');
  };

  const joinChat = (e) => {
    e.preventDefault();
    if (!username) {
      alert('Please enter a username');
      return;
    }
    
    connectWebSocket();
  };

  return (
    <div className="App">
      <header className="App-header">
        <h1>Simple WebSocket Chat</h1>
        <div className="connection-status">
          Status: {connected ? '🟢 Connected' : '🔴 Disconnected'}
        </div>
      </header>
      
      {!isJoined ? (
        <div className="join-container">
          <h2>Join the Chat</h2>
          <form onSubmit={joinChat} className="join-form">
            <input
              type="text"
              placeholder="Enter your username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
            <button type="submit">Join Chat</button>
          </form>
        </div>
      ) : (
        <main className="chat-container">
          <div className="chat-sidebar">
            <div className="room-header">
              <h3>Simple Chat Room</h3>
            </div>
            <h3>Connected Users ({onlineUsers.length})</h3>
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
