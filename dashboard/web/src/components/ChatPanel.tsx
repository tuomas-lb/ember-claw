import { useEffect, useRef, useState, useCallback, KeyboardEvent } from 'react';
import { connectChat, fetchMessages, ChatMessage } from '../api/client';

interface Props {
  instanceName: string;
}

// A stable session id per instance, persisted in localStorage so the
// conversation (and the agent's own session continuity) survives navigation,
// reloads, and reconnects. Server persists all messages under this id.
function stableSessionKey(instanceName: string): string {
  const storageKey = `eclaw.chat.session.${instanceName}`;
  let key = localStorage.getItem(storageKey);
  if (!key) {
    key = `web-${crypto.randomUUID()}`;
    localStorage.setItem(storageKey, key);
  }
  return key;
}

interface MsgEntry {
  id: number;
  role: 'user' | 'agent';
  text: string;
  streaming?: boolean;
}

type WsState = 'connecting' | 'connected' | 'disconnected' | 'error';

let _msgId = 0;

export default function ChatPanel({ instanceName }: Props) {
  const [messages, setMessages] = useState<MsgEntry[]>([]);
  const [input, setInput] = useState('');
  const [wsState, setWsState] = useState<WsState>('connecting');
  const [isTyping, setIsTyping] = useState(false);
  const [sessionKey] = useState<string>(() => stableSessionKey(instanceName));
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const unmounted = useRef(false);

  const appendAgentChunk = useCallback((text: string, done: boolean) => {
    setMessages(prev => {
      const last = prev[prev.length - 1];
      if (last && last.role === 'agent' && last.streaming) {
        return [
          ...prev.slice(0, -1),
          { ...last, text: last.text + text, streaming: !done },
        ];
      }
      return [
        ...prev,
        { id: ++_msgId, role: 'agent', text, streaming: !done },
      ];
    });
    if (done) setIsTyping(false);
  }, []);

  const connect = useCallback(() => {
    if (unmounted.current) return;
    setWsState('connecting');

    const ws = connectChat(instanceName);
    wsRef.current = ws;

    ws.onopen = () => setWsState('connected');
    ws.onerror = () => setWsState('error');
    ws.onclose = () => {
      if (unmounted.current) return;
      setWsState('disconnected');
      reconnectTimer.current = setTimeout(connect, 3000);
    };
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data as string) as ChatMessage;
        if (msg.error) {
          appendAgentChunk(`[error: ${msg.error}]`, true);
          setIsTyping(false);
          return;
        }
        appendAgentChunk(msg.text, msg.done);
      } catch {
        // raw text fallback
        appendAgentChunk(e.data as string, false);
      }
    };
  }, [instanceName, appendAgentChunk]);

  // Load persisted history for this instance's session on mount, then connect.
  useEffect(() => {
    unmounted.current = false;
    let cancelled = false;
    fetchMessages(instanceName, sessionKey)
      .then(stored => {
        if (cancelled) return;
        setMessages(stored.map(m => ({
          id: ++_msgId,
          role: m.role,
          text: m.content,
        })));
      })
      .catch(() => { /* history is best-effort; start empty */ })
      .finally(() => {
        if (!cancelled) setHistoryLoaded(true);
      });
    connect();
    return () => {
      cancelled = true;
      unmounted.current = true;
      wsRef.current?.close();
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    };
  }, [connect, instanceName, sessionKey]);

  // Auto-scroll on new messages
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, isTyping]);

  function send() {
    const text = input.trim();
    if (!text || wsState !== 'connected') return;

    setMessages(prev => [
      ...prev,
      { id: ++_msgId, role: 'user', text },
    ]);
    setInput('');
    setIsTyping(true);

    const payload = { message: text, session_key: sessionKey };
    wsRef.current?.send(JSON.stringify(payload));
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  }

  const wsDotClass =
    wsState === 'connected' ? 'connected' :
    wsState === 'connecting' ? 'connecting' :
    'error';

  return (
    <div className="chat-container">
      <div className="chat-messages" ref={scrollRef}>
        {messages.length === 0 && historyLoaded && (
          <div style={{ color: 'var(--text-faint)', fontSize: 12, textAlign: 'center', padding: 24 }}>
            Send a message to start a conversation
          </div>
        )}
        {messages.map(msg => (
          <div
            key={msg.id}
            className={`chat-msg chat-msg-${msg.role}`}
          >
            <div className="chat-bubble">
              {msg.text}
              {msg.streaming && (
                <span style={{ opacity: 0.5, animation: 'none' }}>▊</span>
              )}
            </div>
            <div className="chat-meta">
              {msg.role === 'user' ? 'you' : instanceName}
            </div>
          </div>
        ))}
        {isTyping && !messages.some(m => m.role === 'agent' && m.streaming) && (
          <div className="chat-typing">typing...</div>
        )}
      </div>

      <div className="chat-status-bar">
        <span className={`ws-dot ${wsDotClass}`} />
        <span>
          {wsState === 'connected' && `connected — session ${sessionKey.slice(0, 8)}...`}
          {wsState === 'connecting' && 'connecting...'}
          {wsState === 'disconnected' && 'disconnected — reconnecting...'}
          {wsState === 'error' && 'connection error'}
        </span>
      </div>

      <div className="chat-input-row">
        <textarea
          className="chat-input"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={
            wsState === 'connected'
              ? 'Type a message... (Enter to send, Shift+Enter for newline)'
              : wsState === 'connecting'
              ? 'Connecting...'
              : 'Disconnected'
          }
          disabled={wsState !== 'connected'}
          rows={1}
        />
        <button
          className="btn btn-primary"
          onClick={send}
          disabled={wsState !== 'connected' || !input.trim()}
        >
          send
        </button>
      </div>
    </div>
  );
}
