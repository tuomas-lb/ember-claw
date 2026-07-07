import { useEffect, useRef, useState, useCallback, KeyboardEvent } from 'react';
import { connectChat, fetchMessages, ChatEvent, ChatStep } from '../api/client';

interface Props {
  instanceName: string;
}

// A stable session id per instance, persisted in localStorage so the
// conversation (and the agent's own session continuity) survives navigation,
// reloads, and reconnects. The server persists all messages under this id and
// tracks the live turn per session.
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
  steps?: ChatStep[]; // when role === 'thinking'
}

// Live is the current in-flight turn as reported by the server. null = idle.
interface Live {
  message: string;
  steps: ChatStep[];
}

type WsState = 'connecting' | 'connected' | 'disconnected' | 'error';

let _msgId = 0;

export default function ChatPanel({ instanceName }: Props) {
  const [messages, setMessages] = useState<MsgEntry[]>([]);
  const [live, setLive] = useState<Live | null>(null);
  const [queued, setQueued] = useState<string[]>([]);
  const [input, setInput] = useState('');
  const [wsState, setWsState] = useState<WsState>('connecting');
  const [historyLoaded, setHistoryLoaded] = useState(false);
  const [sessionKey] = useState<string>(() => stableSessionKey(instanceName));
  const wsRef = useRef<WebSocket | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const unmounted = useRef(false);

  // Load persisted history (completed turns) — replaces the message list with
  // the server's authoritative copy. The live turn is tracked separately.
  const loadHistory = useCallback(() => {
    fetchMessages(instanceName, sessionKey)
      .then(stored => {
        if (unmounted.current) return;
        setMessages(stored.map(m => {
          if (m.role === 'thinking') {
            let steps: ChatStep[] = [];
            try { steps = JSON.parse(m.content) as ChatStep[]; } catch { /* ignore */ }
            return { id: ++_msgId, role: 'thinking' as const, text: '', steps };
          }
          return { id: ++_msgId, role: m.role as 'user' | 'agent', text: m.content };
        }));
      })
      .catch(() => { /* best-effort */ })
      .finally(() => { if (!unmounted.current) setHistoryLoaded(true); });
  }, [instanceName, sessionKey]);

  const connect = useCallback(() => {
    if (unmounted.current) return;
    setWsState('connecting');
    const ws = connectChat(instanceName, sessionKey);
    wsRef.current = ws;

    ws.onopen = () => setWsState('connected');
    ws.onerror = () => setWsState('error');
    ws.onclose = () => {
      if (unmounted.current) return;
      setWsState('disconnected');
      reconnectTimer.current = setTimeout(connect, 2000);
    };
    ws.onmessage = (e) => {
      let ev: ChatEvent;
      try { ev = JSON.parse(e.data as string) as ChatEvent; } catch { return; }
      switch (ev.type) {
        case 'snapshot':
          // Authoritative state of the in-flight turn on (re)connect.
          setLive(ev.running ? { message: ev.message ?? '', steps: ev.steps ?? [] } : null);
          setQueued(ev.queue ?? []);
          break;
        case 'status':
          setQueued(ev.queue ?? []);
          if (ev.running) {
            setLive(l => l ?? { message: ev.message ?? '', steps: [] });
          } else {
            // Turn(s) finished / idle — pull the authoritative history.
            setLive(null);
            loadHistory();
          }
          break;
        case 'step':
          if (ev.step) {
            const s = ev.step;
            setLive(l => ({ message: l?.message ?? '', steps: [...(l?.steps ?? []), s] }));
          }
          break;
        case 'done':
          setLive(null);
          loadHistory();
          break;
        case 'error':
          setLive(null);
          loadHistory();
          if (ev.error) {
            setMessages(prev => [...prev, { id: ++_msgId, role: 'agent', text: `[error: ${ev.error}]` }]);
          }
          break;
      }
    };
  }, [instanceName, sessionKey, loadHistory]);

  useEffect(() => {
    unmounted.current = false;
    loadHistory();
    connect();
    // Re-sync history when returning to the tab (in case something completed
    // while the socket was backgrounded).
    const onFocus = () => loadHistory();
    window.addEventListener('focus', onFocus);
    return () => {
      unmounted.current = true;
      window.removeEventListener('focus', onFocus);
      wsRef.current?.close();
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    };
  }, [connect, loadHistory]);

  // Auto-scroll to the newest content.
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, live]);

  function send() {
    const text = input.trim();
    if (!text || wsState !== 'connected') return;
    // Optimistically show the running turn immediately (only if idle — a queued
    // message surfaces when its turn actually starts, via the server's status
    // event). The server confirms via status/step and persists on completion.
    setLive(l => l ?? { message: text, steps: [] });
    setInput('');
    wsRef.current?.send(JSON.stringify({ message: text, session_key: sessionKey }));
  }

  function abort() {
    wsRef.current?.send(JSON.stringify({ action: 'abort' }));
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

  const lastStep = live?.steps[live.steps.length - 1];
  const activity = lastStep
    ? (lastStep.kind === 'tool' ? `running ${lastStep.tool}…` : 'thinking…')
    : 'working…';

  const empty = messages.length === 0 && !live;

  return (
    <div className="chat-container">
      <div className="chat-messages" ref={scrollRef}>
        {empty && !historyLoaded && (
          <div className="chat-empty">loading conversation…</div>
        )}
        {empty && historyLoaded && (
          <div className="chat-empty">Send a message to start a conversation</div>
        )}

        {messages.map(msg => (
          msg.role === 'thinking' ? (
            <details key={msg.id} className="chat-thinking" open>
              <summary>💭 thinking · {msg.steps?.length ?? 0} step{(msg.steps?.length ?? 0) === 1 ? '' : 's'}</summary>
              <div className="chat-steps">{(msg.steps ?? []).map(renderStep)}</div>
            </details>
          ) : (
            <div key={msg.id} className={`chat-msg chat-msg-${msg.role}`}>
              <div className="chat-bubble">{msg.text}</div>
              <div className="chat-meta">{msg.role === 'user' ? 'you' : instanceName}</div>
            </div>
          )
        ))}

        {/* Live, server-tracked in-flight turn (survives reload/second tab). */}
        {live && (
          <>
            {live.message && (
              <div className="chat-msg chat-msg-user">
                <div className="chat-bubble">{live.message}</div>
                <div className="chat-meta">you</div>
              </div>
            )}
            <div className="chat-working">
              <div className="chat-working-head">
                <span className="chat-working-spinner" />
                <span>{instanceName} is {activity}</span>
                <button className="chat-stop" onClick={abort} title="Stop the current turn">stop</button>
              </div>
              {live.steps.length > 0 && (
                <div className="chat-steps">{live.steps.map(renderStep)}</div>
              )}
            </div>
          </>
        )}

        {/* Messages typed while a turn is running — queued server-side. */}
        {queued.map((q, i) => (
          <div key={`q-${i}`} className="chat-msg chat-msg-user chat-msg-queued">
            <div className="chat-bubble">{q}</div>
            <div className="chat-meta">queued</div>
          </div>
        ))}
      </div>

      <div className="chat-status-bar">
        <span className={`ws-dot ${wsDotClass}`} />
        <span>
          {wsState === 'connected' && (live ? `${instanceName} is working…` : `connected — session ${sessionKey.slice(0, 12)}…`)}
          {wsState === 'connecting' && 'connecting…'}
          {wsState === 'disconnected' && 'reconnecting…'}
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
              ? 'Type a message… (Enter to send, Shift+Enter for newline)'
              : 'Connecting…'
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
