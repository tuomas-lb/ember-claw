import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { listInstances, getFleetMD, putFleetMD, Instance } from '../api/client';
import StatusBadge from './StatusBadge';

interface AgentRole {
  name: string;
  role: string;
  tier: string;
  handoff: string;
  outputs: string;
}

function parseFleetMD(md: string): AgentRole[] {
  const roles: AgentRole[] = [];
  const lines = md.split('\n');
  for (const line of lines) {
    if (!line.startsWith('|') || line.includes('---') || line.includes('Name')) continue;
    const cols = line.split('|').map(c => c.trim()).filter(Boolean);
    if (cols.length >= 5) {
      roles.push({
        name: cols[0], role: cols[1], tier: cols[2],
        handoff: cols[3], outputs: cols[4],
      });
    }
  }
  return roles;
}

function rolesToFleetMD(roles: AgentRole[]): string {
  let md = '# Fleet Registry\n\n';
  md += '| Name | Role | Tier | Handoff Route | Output Files |\n';
  md += '|------|------|------|---------------|--------------|\n';
  for (const r of roles) {
    md += `| ${r.name} | ${r.role} | ${r.tier} | ${r.handoff} | ${r.outputs} |\n`;
  }
  return md;
}

const ROLES = ['Orchestrator', 'Worker', 'Observer', 'Canary'];
const TIERS = ['Primary', 'Investigation', 'Implementation', 'Writing', 'QA', 'Health', 'Custom'];

export default function FleetEditor() {
  const { data: instances } = useQuery({
    queryKey: ['instances'],
    queryFn: listInstances,
    refetchInterval: 10000,
  });

  const [roles, setRoles] = useState<AgentRole[]>([]);
  const [original, setOriginal] = useState<AgentRole[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: 'ok' | 'err'; text: string } | null>(null);
  const [editing, setEditing] = useState<string | null>(null);

  useEffect(() => {
    getFleetMD()
      .then((md) => {
        const parsed = parseFleetMD(md);
        setRoles(parsed);
        setOriginal(parsed);
      })
      .catch((err) => setMessage({ type: 'err', text: err.message }))
      .finally(() => setLoading(false));
  }, []);

  const dirty = JSON.stringify(roles) !== JSON.stringify(original);

  const assignedNames = new Set(roles.map(r => r.name));
  const unassigned = (instances || []).filter(i => !assignedNames.has(i.name));

  function updateRole(name: string, field: keyof AgentRole, value: string) {
    setRoles(prev => prev.map(r => r.name === name ? { ...r, [field]: value } : r));
  }

  function removeRole(name: string) {
    setRoles(prev => prev.filter(r => r.name !== name));
    setEditing(null);
  }

  function addInstance(inst: Instance) {
    setRoles(prev => [...prev, {
      name: inst.name,
      role: 'Worker',
      tier: 'Primary',
      handoff: '-> Orchestrator',
      outputs: 'findings/',
    }]);
  }

  async function save() {
    setSaving(true);
    setMessage(null);
    try {
      await putFleetMD(rolesToFleetMD(roles));
      setOriginal([...roles]);
      setMessage({ type: 'ok', text: 'Fleet registry saved.' });
    } catch (err: any) {
      setMessage({ type: 'err', text: err.message });
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <div className="loading">Loading fleet...</div>;

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Fleet Registry</div>
          <div className="page-subtitle">
            Agent roles and handoff routing. Mounted at <code>/workspace/Fleet.md</code> in all instances.
          </div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {dirty && <span className="fleet-dirty">unsaved</span>}
          <button onClick={save} disabled={!dirty || saving} className="btn btn-primary">
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      {message && (
        <div className={`toast ${message.type === 'ok' ? 'toast-ok' : 'toast-err'}`}>
          {message.text}
        </div>
      )}

      <div className="fleet-grid">
        {roles.map(agent => {
          const inst = (instances || []).find(i => i.name === agent.name);
          const isEditing = editing === agent.name;

          return (
            <div key={agent.name} className={`fleet-card ${isEditing ? 'fleet-card-editing' : ''}`}>
              <div className="fleet-card-header">
                <div className="fleet-card-name">
                  {agent.name}
                  {inst && <StatusBadge status={inst.status} ready={inst.ready} />}
                  {!inst && <span className="badge badge-dim">not deployed</span>}
                </div>
                <button
                  className="btn-icon"
                  onClick={() => setEditing(isEditing ? null : agent.name)}
                  title={isEditing ? 'Done' : 'Edit'}
                >
                  {isEditing ? '✓' : '✎'}
                </button>
              </div>

              {!isEditing ? (
                <div className="fleet-card-body">
                  <div className="fleet-card-row">
                    <span className="fleet-label">Role</span>
                    <span className="fleet-value">{agent.role}</span>
                  </div>
                  <div className="fleet-card-row">
                    <span className="fleet-label">Tier</span>
                    <span className="fleet-value">{agent.tier}</span>
                  </div>
                  <div className="fleet-card-row">
                    <span className="fleet-label">Handoff</span>
                    <span className="fleet-value mono-dim">{agent.handoff}</span>
                  </div>
                  <div className="fleet-card-row">
                    <span className="fleet-label">Outputs</span>
                    <span className="fleet-value mono-dim">{agent.outputs}</span>
                  </div>
                  {inst && (
                    <div className="fleet-card-row">
                      <span className="fleet-label">Model</span>
                      <span className="fleet-value">{inst.model || '—'}</span>
                    </div>
                  )}
                </div>
              ) : (
                <div className="fleet-card-body">
                  <div className="fleet-edit-row">
                    <label>Role</label>
                    <select value={agent.role} onChange={e => updateRole(agent.name, 'role', e.target.value)} className="form-select-sm">
                      {ROLES.map(r => <option key={r} value={r}>{r}</option>)}
                    </select>
                  </div>
                  <div className="fleet-edit-row">
                    <label>Tier</label>
                    <select value={agent.tier} onChange={e => updateRole(agent.name, 'tier', e.target.value)} className="form-select-sm">
                      {TIERS.map(t => <option key={t} value={t}>{t}</option>)}
                    </select>
                  </div>
                  <div className="fleet-edit-row">
                    <label>Handoff</label>
                    <input
                      value={agent.handoff}
                      onChange={e => updateRole(agent.name, 'handoff', e.target.value)}
                      className="form-input-sm"
                      placeholder="-> AgentName"
                    />
                  </div>
                  <div className="fleet-edit-row">
                    <label>Outputs</label>
                    <input
                      value={agent.outputs}
                      onChange={e => updateRole(agent.name, 'outputs', e.target.value)}
                      className="form-input-sm"
                      placeholder="findings/, reports/"
                    />
                  </div>
                  <button className="btn btn-danger-sm" onClick={() => removeRole(agent.name)}>
                    Remove from fleet
                  </button>
                </div>
              )}
            </div>
          );
        })}

        {/* Unassigned instances */}
        {unassigned.map(inst => (
          <div key={inst.name} className="fleet-card fleet-card-unassigned">
            <div className="fleet-card-header">
              <div className="fleet-card-name">
                {inst.name}
                <StatusBadge status={inst.status} ready={inst.ready} />
              </div>
            </div>
            <div className="fleet-card-body">
              <div className="fleet-card-row">
                <span className="fleet-label">Model</span>
                <span className="fleet-value">{inst.model || '—'}</span>
              </div>
              <div className="fleet-unassigned-hint">Not in fleet registry</div>
              <button className="btn btn-outline-sm" onClick={() => addInstance(inst)}>
                Add to fleet
              </button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
