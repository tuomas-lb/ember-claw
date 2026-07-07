import { useEffect, useRef, useState, useCallback } from 'react';
import { connectLogs } from '../api/client';

interface Props {
  instanceName: string;
}

type WsState = 'connecting' | 'connected' | 'disconnected' | 'error';

function classifyLine(line: string): string {
  const l = line.toLowerCase();
  if (l.includes('error') || l.includes('fatal') || l.includes('panic')) return 'log-line-error';
  if (l.includes('warn')) return 'log-line-warn';
  if (l.includes('info')) return 'log-line-info';
  return '';
}

export default function LogViewer({ instanceName }: Props) {
  const [lines, setLines] = useState<string[]>([]);
  const [wsState, setWsState] = useState<WsState>('connecting');
  const [paused, setPaused] = useState(false);
  const bodyRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(paused);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const unmounted = useRef(false);

  pausedRef.current = paused;

  const connect = useCallback(() => {
    if (unmounted.current) return;
    setWsState('connecting');
    const ws = connectLogs(instanceName, (line) => {
      if (!pausedRef.current) {
        setLines(prev => {
          const next = [...prev, line];
          return next.length > 2000 ? next.slice(next.length - 2000) : next;
        });
      }
    }, true);

    ws.onopen = () => setWsState('connected');
    ws.onerror = () => setWsState('error');
    ws.onclose = () => {
      if (unmounted.current) return;
      setWsState('disconnected');
      reconnectTimer.current = setTimeout(connect, 3000);
    };
    wsRef.current = ws;
  }, [instanceName]);

  useEffect(() => {
    unmounted.current = false;
    connect();
    return () => {
      unmounted.current = true;
      wsRef.current?.close();
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    };
  }, [connect]);

  // Auto-scroll
  useEffect(() => {
    if (!paused && bodyRef.current) {
      bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
    }
  }, [lines, paused]);

  function clear() {
    setLines([]);
  }

  const wsDotClass =
    wsState === 'connected' ? 'connected' :
    wsState === 'connecting' ? 'connecting' :
    'error';

  return (
    <div className="terminal">
      <div className="terminal-toolbar">
        <span className="terminal-toolbar-title">
          logs — {instanceName}
        </span>
        <button
          className="btn btn-outline btn-sm"
          onClick={() => setPaused(p => !p)}
        >
          {paused ? 'resume' : 'pause'}
        </button>
        <button className="btn btn-outline btn-sm" onClick={clear}>
          clear
        </button>
      </div>
      <div className="terminal-body" ref={bodyRef}>
        {lines.length === 0 && (
          <span style={{ color: 'var(--text-faint)' }}>
            {wsState === 'connecting' ? 'connecting...' : 'no output'}
          </span>
        )}
        {lines.map((line, i) => (
          <div key={i} className={`log-line ${classifyLine(line)}`}>
            {line}
          </div>
        ))}
      </div>
      <div className="chat-status-bar">
        <span className={`ws-dot ${wsDotClass}`} />
        <span>
          {wsState === 'connected' && `streaming — ${lines.length} lines`}
          {wsState === 'connecting' && 'connecting...'}
          {wsState === 'disconnected' && 'disconnected — reconnecting...'}
          {wsState === 'error' && 'connection error'}
        </span>
        {paused && <span style={{ color: 'var(--amber)', marginLeft: 8 }}>PAUSED</span>}
      </div>
    </div>
  );
}
