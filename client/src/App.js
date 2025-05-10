import { useState, useEffect, useRef } from 'react';
import './App.css';

function App() {
  const [connected, setConnected] = useState(false);
  const [messages, setMessages] = useState([]);
  const [inputMessage, setInputMessage] = useState('');
  const socketRef = useRef(null);

  useEffect(() => {
    // Initialize WebSocket connection
    const connectWebSocket = () => {
      const ws = new WebSocket('ws://localhost:8080/ws');
      
      ws.onopen = () => {
        console.log('Connected to WebSocket server');
        setConnected(true);
        setMessages(prev => [...prev, { text: 'Connected to server', sender: 'system' }]);
      };
      
      ws.onmessage = (event) => {
        console.log('Message from server:', event.data);
        setMessages(prev => [...prev, { text: event.data, sender: 'server' }]);
      };
      
      ws.onclose = () => {
        console.log('Disconnected from WebSocket server');
        setConnected(false);
        setMessages(prev => [...prev, { text: 'Disconnected from server', sender: 'system' }]);
        
        // Try to reconnect after 3 seconds
        setTimeout(() => {
          if (socketRef.current === ws) {
            connectWebSocket();
          }
        }, 3000);
      };
      
      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        setMessages(prev => [...prev, { text: 'Error connecting to server', sender: 'system' }]);
      };
      
      socketRef.current = ws;
    };
    
    connectWebSocket();
    
    // Clean up on component unmount
    return () => {
      if (socketRef.current) {
        socketRef.current.close();
      }
    };
  }, []);
  
  const sendMessage = (e) => {
    e.preventDefault();
    if (!inputMessage.trim() || !socketRef.current || socketRef.current.readyState !== WebSocket.OPEN) return;
    
    socketRef.current.send(inputMessage);
    setMessages(prev => [...prev, { text: inputMessage, sender: 'client' }]);
    setInputMessage('');
  };

  return (
    <div className="App">
      <header className="App-header">
        <h1>WebSocket Chat Demo</h1>
        <div className="connection-status">
          Status: {connected ? '🟢 Connected' : '🔴 Disconnected'}
        </div>
      </header>
      <main className="chat-container">
        <div className="message-list">
          {messages.map((msg, index) => (
            <div key={index} className={`message ${msg.sender}`}>
              <span className="sender">{msg.sender}: </span>
              <span className="text">{msg.text}</span>
            </div>
          ))}
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
      </main>
    </div>
  );
}

export default App;
