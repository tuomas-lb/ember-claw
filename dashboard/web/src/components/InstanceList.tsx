import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { listInstances, deleteInstance, restartInstance, Instance } from '../api/client';
import StatusBadge from './StatusBadge';

function ConfirmDelete({
  name,
  onConfirm,
  onCancel,
}: {
  name: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
      <span style={{ fontSize: 11, color: 'var(--text-dim)' }}>delete {name}?</span>
      <button className="btn btn-danger btn-sm" onClick={onConfirm}>yes</button>
      <button className="btn btn-outline btn-sm" onClick={onCancel}>no</button>
    </span>
  );
}

function InstanceRow({ instance }: { instance: Instance }) {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [confirmDelete, setConfirmDelete] = useState(false);

  const deleteMut = useMutation({
    mutationFn: () => deleteInstance(instance.name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['instances'] }),
    onError: (e: Error) => alert(`Delete failed: ${e.message}`),
  });

  const restartMut = useMutation({
    mutationFn: () => restartInstance(instance.name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['instances'] }),
    onError: (e: Error) => alert(`Restart failed: ${e.message}`),
  });

  return (
    <tr>
      <td>
        <Link to={`/instances/${instance.name}`} className="instance-name">
          {instance.name}
        </Link>
      </td>
      <td>
        <StatusBadge status={instance.status} ready={instance.ready} />
      </td>
      <td className="text-dim">{instance.model || '—'}</td>
      <td className="text-dim">{instance.provider || '—'}</td>
      <td className="mono-dim">{instance.age || '—'}</td>
      <td className="mono-dim">
        {instance.cpu_limit || '—'} / {instance.memory_limit || '—'}
      </td>
      <td>
        {confirmDelete ? (
          <ConfirmDelete
            name={instance.name}
            onConfirm={() => deleteMut.mutate()}
            onCancel={() => setConfirmDelete(false)}
          />
        ) : (
          <div className="td-actions">
            <button
              className="btn btn-outline btn-sm"
              onClick={() => navigate(`/instances/${instance.name}`)}
            >
              view
            </button>
            <button
              className="btn btn-amber btn-sm"
              onClick={() => restartMut.mutate()}
              disabled={restartMut.isPending}
            >
              {restartMut.isPending ? '...' : 'restart'}
            </button>
            <button
              className="btn btn-danger btn-sm"
              onClick={() => setConfirmDelete(true)}
              disabled={deleteMut.isPending}
            >
              {deleteMut.isPending ? '...' : 'delete'}
            </button>
          </div>
        )}
      </td>
    </tr>
  );
}

export default function InstanceList() {
  const { data, isLoading, isError, error, dataUpdatedAt } = useQuery({
    queryKey: ['instances'],
    queryFn: listInstances,
    refetchInterval: 5000,
  });

  const lastUpdate = dataUpdatedAt
    ? new Date(dataUpdatedAt).toLocaleTimeString()
    : null;

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Instances</div>
          {lastUpdate && (
            <div className="page-subtitle">last updated {lastUpdate}</div>
          )}
        </div>
        <Link to="/deploy" className="btn btn-primary">
          + Deploy New
        </Link>
      </div>

      {isLoading && <div className="loading">loading instances...</div>}

      {isError && (
        <div className="error-box">
          Failed to load instances: {(error as Error).message}
        </div>
      )}

      {!isLoading && !isError && data && data.length === 0 && (
        <div className="empty-state">
          <span>No instances deployed</span>
          <Link to="/deploy" className="btn btn-primary">Deploy your first instance</Link>
        </div>
      )}

      {!isLoading && !isError && data && data.length > 0 && (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Status</th>
                <th>Model</th>
                <th>Provider</th>
                <th>Age</th>
                <th>CPU / Mem</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map(inst => (
                <InstanceRow key={inst.name} instance={inst} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
