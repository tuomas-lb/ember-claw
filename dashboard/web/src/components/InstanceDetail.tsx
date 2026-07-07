import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getInstance, deleteInstance, restartInstance } from '../api/client';
import StatusBadge from './StatusBadge';
import LogViewer from './LogViewer';
import ChatPanel from './ChatPanel';
import ConfigEditor from './ConfigEditor';

type Tab = 'overview' | 'logs' | 'chat' | 'config';

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return `${h}h ${m}m`;
}

export default function InstanceDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [tab, setTab] = useState<Tab>('overview');
  const [confirmDelete, setConfirmDelete] = useState(false);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ['instance', name],
    queryFn: () => getInstance(name!),
    refetchInterval: tab === 'overview' ? 5000 : false,
    enabled: !!name,
  });

  const deleteMut = useMutation({
    mutationFn: () => deleteInstance(name!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['instances'] });
      navigate('/');
    },
    onError: (e: Error) => alert(`Delete failed: ${e.message}`),
  });

  const restartMut = useMutation({
    mutationFn: () => restartInstance(name!),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['instance', name] }),
    onError: (e: Error) => alert(`Restart failed: ${e.message}`),
  });

  if (!name) return <div className="error-box">No instance name in URL</div>;
  if (isLoading) return <div className="loading">loading instance...</div>;
  if (isError) return <div className="error-box">Failed to load: {(error as Error).message}</div>;
  if (!data) return null;

  return (
    <div>
      {/* Header */}
      <div className="instance-header">
        <div className="instance-header-info">
          <div className="instance-header-title">
            {data.name}
            <StatusBadge status={data.status} ready={data.ready} />
          </div>
          <div className="instance-header-meta">
            <span>model: <span style={{ color: 'var(--text)' }}>{data.model || data.grpc_status?.model || '—'}</span></span>
            <span>provider: <span style={{ color: 'var(--text)' }}>{data.provider || data.grpc_status?.provider || '—'}</span></span>
            {data.pod_name && (
              <span>pod: <span className="mono-dim">{data.pod_name}</span></span>
            )}
          </div>
        </div>

        <div className="instance-header-actions">
          <button
            className="btn btn-amber"
            onClick={() => restartMut.mutate()}
            disabled={restartMut.isPending}
          >
            {restartMut.isPending ? 'restarting...' : 'restart'}
          </button>
          {confirmDelete ? (
            <>
              <span style={{ fontSize: 11, color: 'var(--text-dim)' }}>sure?</span>
              <button
                className="btn btn-danger"
                onClick={() => deleteMut.mutate()}
                disabled={deleteMut.isPending}
              >
                {deleteMut.isPending ? '...' : 'yes, delete'}
              </button>
              <button className="btn btn-outline" onClick={() => setConfirmDelete(false)}>
                cancel
              </button>
            </>
          ) : (
            <button className="btn btn-danger" onClick={() => setConfirmDelete(true)}>
              delete
            </button>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="tabs">
        {(['overview', 'logs', 'chat', 'config'] as Tab[]).map(t => (
          <button
            key={t}
            className={`tab ${tab === t ? 'active' : ''}`}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>

      {/* Overview */}
      {tab === 'overview' && (
        <div>
          <div className="card">
            <div className="card-title">Pod Details</div>
            <div className="detail-grid">
              <div className="detail-item">
                <span className="detail-label">Pod Name</span>
                <span className="detail-value mono-dim">{data.pod_name || '—'}</span>
              </div>
              <div className="detail-item">
                <span className="detail-label">Pod IP</span>
                <span className="detail-value mono-dim">{data.pod_ip || '—'}</span>
              </div>
              <div className="detail-item">
                <span className="detail-label">Status</span>
                <span className="detail-value"><StatusBadge status={data.status} /></span>
              </div>
              <div className="detail-item">
                <span className="detail-label">Age</span>
                <span className="detail-value">{data.age || '—'}</span>
              </div>
              <div className="detail-item">
                <span className="detail-label">CPU Limit</span>
                <span className="detail-value">{data.cpu_limit || '—'}</span>
              </div>
              <div className="detail-item">
                <span className="detail-label">Memory Limit</span>
                <span className="detail-value">{data.memory_limit || '—'}</span>
              </div>
            </div>
          </div>

          {data.grpc_status && (
            <div className="card mt-12">
              <div className="card-title">gRPC Status</div>
              <div className="detail-grid">
                <div className="detail-item">
                  <span className="detail-label">Ready</span>
                  <span className="detail-value">
                    {data.grpc_status.ready
                      ? <span className="text-green">yes</span>
                      : <span className="text-red">no</span>}
                  </span>
                </div>
                <div className="detail-item">
                  <span className="detail-label">Model</span>
                  <span className="detail-value">{data.grpc_status.model || '—'}</span>
                </div>
                <div className="detail-item">
                  <span className="detail-label">Provider</span>
                  <span className="detail-value">{data.grpc_status.provider || '—'}</span>
                </div>
                <div className="detail-item">
                  <span className="detail-label">Uptime</span>
                  <span className="detail-value">
                    {formatUptime(data.grpc_status.uptime_seconds)}
                  </span>
                </div>
              </div>
            </div>
          )}

          {!data.grpc_status && (
            <div className="card mt-12" style={{ color: 'var(--text-dim)' }}>
              <div className="card-title">gRPC Status</div>
              <span style={{ fontSize: 12 }}>
                {data.ready
                  ? 'gRPC status not available'
                  : 'Instance not ready — gRPC status unavailable'}
              </span>
            </div>
          )}
        </div>
      )}

      {tab === 'logs' && <LogViewer instanceName={name} />}
      {tab === 'chat' && <ChatPanel instanceName={name} />}
      {tab === 'config' && <ConfigEditor instanceName={name} />}
    </div>
  );
}
