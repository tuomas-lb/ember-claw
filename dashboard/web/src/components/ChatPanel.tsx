import { useEffect, useRef, useState, useCallback, KeyboardEvent } from 'react';
import { connectChat, fetchMessages, ChatMessage, ChatStep } from '../api/client';

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
  role: 'user' | 'agent' | 'thinking';
  text: string;
  streaming?: boolean;
  steps?: ChatStep[]; // set when role === 'thinking'
}

type WsState = 'connecting' | 'connected' | 'disconnected' | 'error';

let _msgId = 0;

export default function ChatPanel({ instanceName }: Props) {
  const [messages, setMessages] = useState<MsgEntry[]>([]);
  const [input, setInput] = useState('');
  const [wsState, setWsState] = useState<WsState>('connecting');
  const [isTyping, setIsTyping] = useState(false);
  const [steps, setSteps] = useState<ChatStep[]>([]);
  const [sessionKey] = useState<string>(() => stableSessionKey(instanceName));
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const unmounted = useRef(false);
  // Source of truth for the current turn's steps (read at 'done' to commit a
  // persisted-style thinking block); mirrored to `steps` state for live render.
  const liveSteps = useRef<ChatStep[]>([]);

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
        // Intermediate processing step (reasoning / tool call) — show live.
        if (msg.step) {
          liveSteps.current = [...liveSteps.current, msg.step as ChatStep];
          setSteps(liveSteps.current);
          return;
        }
        // On a terminal frame (answer or error), commit the accumulated steps
        // as a persisted-style thinking block, then the answer — so the live
        // view matches what a reload shows.
        const commitThinking = () => {
          if (liveSteps.current.length) {
            const committed = liveSteps.current;
            setMessages(prev => [...prev, { id: ++_msgId, role: 'thinking', text: '', steps: committed }]);
          }
          liveSteps.current = [];
          setSteps([]);
        };
        if (msg.error) {
          commitThinking();
          appendAgentChunk(`[error: ${msg.error}]`, true);
          setIsTyping(false);
          return;
        }
        if (msg.done) {
          commitThinking();
        }
        appendAgentChunk(msg.text, msg.done);
      } catch {
        // raw text fallback
        appendAgentChunk(e.data as string, false);
      }
    };
  }, [instanceName, appendAgentChunk]);

  // Load persisted history for this instance's session. Replaces the message
  // list with the server's copy, but never while a turn is streaming in this
  // view (that would clobber the in-flight response).
  const loadHistory = useCallback(() => {
    if (isTyping) return;
    fetchMessages(instanceName, sessionKey)
      .then(stored => {
        if (unmounted.current || isTyping) return;
        setMessages(stored.map(m => {
          if (m.role === 'thinking') {
            let steps: ChatStep[] = [];
            try { steps = JSON.parse(m.content) as ChatStep[]; } catch { /* ignore */ }
            return { id: ++_msgId, role: 'thinking' as const, text: '', steps };
          }
          return { id: ++_msgId, role: m.role as 'user' | 'agent', text: m.content };
        }));
      })
      .catch(() => { /* history is best-effort */ })
      .finally(() => {
        if (!unmounted.current) setHistoryLoaded(true);
      });
  }, [instanceName, sessionKey, isTyping]);

  // Load history on mount, then connect. Also refetch whenever the window
  // regains focus — so a response that completed server-side while the user
  // was on another tab reappears on return (the live stream only reaches the
  // tab that was open when it was generated).
  useEffect(() => {
    unmounted.current = false;
    loadHistory();
    connect();
    const onFocus = () => loadHistory();
    window.addEventListener('focus', onFocus);
    return () => {
      unmounted.current = true;
      window.removeEventListener('focus', onFocus);
      wsRef.current?.close();
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    };
    // loadHistory intentionally omitted: it depends on isTyping, which would
    // otherwise tear down and reopen the WebSocket on every send.
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
    liveSteps.current = [];
    setSteps([]);

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

  const renderStep = (s: ChatStep, i: number) => (
    <div key={i} className={`chat-step chat-step-${s.kind}`}>
      {s.kind === 'tool' ? (
        <>
          <span className="chat-step-icon">⚙</span>
          <span className="chat-step-tool">{s.tool}</span>
          {s.content && <span className="chat-step-args">{s.content}</span>}
        </>
      ) : (
        <>
          <span className="chat-step-icon">✳</span>
          <span className="chat-step-reasoning">{s.content}</span>
        </>
      )}
    </div>
  );

  return (
    <div className="chat-container">
      <div className="chat-messages" ref={scrollRef}>
        {messages.length === 0 && !historyLoaded && (
          <div style={{ color: 'var(--text-faint)', fontSize: 12, textAlign: 'center', padding: 24 }}>
            loading conversation…
          </div>
        )}
        {messages.length === 0 && historyLoaded && (
          <div style={{ color: 'var(--text-faint)', fontSize: 12, textAlign: 'center', padding: 24 }}>
            Send a message to start a conversation
          </div>
        )}
        {messages.map(msg => (
          msg.role === 'thinking' ? (
            <details key={msg.id} className="chat-thinking">
              <summary>💭 thinking · {msg.steps?.length ?? 0} step{(msg.steps?.length ?? 0) === 1 ? '' : 's'}</summary>
              <div className="chat-steps">
                {(msg.steps ?? []).map(renderStep)}
              </div>
            </details>
          ) : (
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
          )
        ))}
        {steps.length > 0 && (
          <div className="chat-steps">
            {steps.map(renderStep)}
          </div>
        )}
        {isTyping && !messages.some(m => m.role === 'agent' && m.streaming) && (
          <div className="chat-typing">{steps.length > 0 ? 'working...' : 'typing...'}</div>
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
