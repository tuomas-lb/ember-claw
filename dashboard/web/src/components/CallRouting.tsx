import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getCallRouting, putCallRouting, listInstances } from '../api/client';

const VOICES = ['Puck', 'Charon', 'Kore', 'Fenrir', 'Aoede', 'Leda', 'Orus', 'Zephyr'];

interface RouteEntry {
  pattern: string;  // "default", "ext:1001", "did:+358*", "caller:+1*"
  target: string;   // PicoClaw instance name
}

interface InstanceEntry {
  name: string;
  answer: string;
  maxConcurrent: number;
  greeting: string;
  voice: string;
}

function parseConfig(data: Record<string, string>): { routes: RouteEntry[]; instances: InstanceEntry[] } {
  const instanceMap: Record<string, InstanceEntry> = {};
  const routes: RouteEntry[] = [];

  for (const [key, val] of Object.entries(data)) {
    if (key.startsWith('instance.')) {
      const parts = key.split('.');
      if (parts.length !== 3) continue;
      const name = parts[1], field = parts[2];
      if (!instanceMap[name]) {
        instanceMap[name] = { name, answer: 'all', maxConcurrent: 3, greeting: 'Hello, how can I help you?', voice: 'Puck' };
      }
      const inst = instanceMap[name];
      if (field === 'answer') inst.answer = val;
      if (field === 'max_concurrent') inst.maxConcurrent = parseInt(val) || 3;
      if (field === 'greeting') inst.greeting = val;
      if (field === 'voice') inst.voice = val;
    }
  }

  const rulesStr = data['routing.rules'] || '';
  for (const line of rulesStr.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const [pattern, target] = trimmed.split('->').map(s => s.trim());
    if (pattern && target) routes.push({ pattern, target });
  }

  return { routes, instances: Object.values(instanceMap) };
}

function toConfigMap(routes: RouteEntry[], instances: InstanceEntry[]): Record<string, string> {
  const data: Record<string, string> = {};
  for (const inst of instances) {
    data[`instance.${inst.name}.answer`] = inst.answer;
    data[`instance.${inst.name}.max_concurrent`] = String(inst.maxConcurrent);
    data[`instance.${inst.name}.greeting`] = inst.greeting;
    data[`instance.${inst.name}.voice`] = inst.voice;
  }
  data['routing.rules'] = routes.map(r => `${r.pattern} -> ${r.target}`).join('\n');
  return data;
}

export default function CallRouting() {
  const { data: liveInstances } = useQuery({ queryKey: ['instances'], queryFn: listInstances });

  const [routes, setRoutes] = useState<RouteEntry[]>([]);
  const [instances, setInstances] = useState<InstanceEntry[]>([]);
  const [original, setOriginal] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: 'ok' | 'err'; text: string } | null>(null);

  useEffect(() => {
    getCallRouting()
      .then(data => {
        const parsed = parseConfig(data);
        setRoutes(parsed.routes);
        setInstances(parsed.instances);
        setOriginal(JSON.stringify(parsed));
      })
      .catch(err => setMessage({ type: 'err', text: err.message }))
      .finally(() => setLoading(false));
  }, []);

  const dirty = JSON.stringify({ routes, instances }) !== original;
  const instanceNames = (liveInstances || []).map(i => i.name);
  const configuredNames = new Set(instances.map(i => i.name));

  function updateInstance(name: string, field: keyof InstanceEntry, value: string | number) {
    setInstances(prev => prev.map(i => i.name === name ? { ...i, [field]: value } : i));
  }

  function addInstance(name: string) {
    setInstances(prev => [...prev, { name, answer: 'all', maxConcurrent: 3, greeting: 'Hello, how can I help you?', voice: 'Puck' }]);
    if (!routes.some(r => r.target === name)) {
      setRoutes(prev => [...prev, { pattern: 'default', target: name }]);
    }
  }

  function removeInstance(name: string) {
    setInstances(prev => prev.filter(i => i.name !== name));
    setRoutes(prev => prev.filter(r => r.target !== name));
  }

  function addRoute() {
    setRoutes(prev => [...prev, { pattern: 'default', target: instances[0]?.name || '' }]);
  }

  function removeRoute(idx: number) {
    setRoutes(prev => prev.filter((_, i) => i !== idx));
  }

  function updateRoute(idx: number, field: keyof RouteEntry, value: string) {
    setRoutes(prev => prev.map((r, i) => i === idx ? { ...r, [field]: value } : r));
  }

  async function save() {
    setSaving(true);
    setMessage(null);
    try {
      await putCallRouting(toConfigMap(routes, instances));
      setOriginal(JSON.stringify({ routes, instances }));
      // Restart voice bridge to pick up new config
      try {
        await fetch('/api/telephony/restart', { method: 'POST' });
        setMessage({ type: 'ok', text: 'Call routing saved and voice bridge restarted.' });
      } catch {
        setMessage({ type: 'ok', text: 'Call routing saved. Voice bridge restart failed — restart manually.' });
      }
    } catch (err: any) {
      setMessage({ type: 'err', text: err.message });
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <div className="loading">Loading routing config...</div>;

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Call Routing</div>
          <div className="page-subtitle">Configure which PicoClaw instances handle incoming calls</div>
        </div>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {dirty && <span className="fleet-dirty">unsaved</span>}
          <button onClick={save} disabled={!dirty || saving} className="btn btn-primary">
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>

      {message && <div className={`toast ${message.type === 'ok' ? 'toast-ok' : 'toast-err'}`}>{message.text}</div>}

      {/* Instance telephony config */}
      <h3 className="section-title">Bot Configuration</h3>
      <div className="routing-grid">
        {instances.map(inst => (
          <div key={inst.name} className="fleet-card">
            <div className="fleet-card-header">
              <div className="fleet-card-name">{inst.name}</div>
              <button className="btn-icon" onClick={() => removeInstance(inst.name)} title="Remove">✕</button>
            </div>
            <div className="fleet-card-body">
              <div className="fleet-edit-row">
                <label>Answer calls</label>
                <select value={inst.answer} onChange={e => updateInstance(inst.name, 'answer', e.target.value)} className="form-select-sm">
                  <option value="all">All routed calls</option>
                  <option value="configured_only">Only matching rules</option>
                  <option value="none">Don't answer (outbound only)</option>
                </select>
              </div>
              <div className="fleet-edit-row">
                <label>Max concurrent calls</label>
                <input type="number" min={1} max={20} value={inst.maxConcurrent}
                  onChange={e => updateInstance(inst.name, 'maxConcurrent', parseInt(e.target.value) || 1)}
                  className="form-input-sm" />
              </div>
              <div className="fleet-edit-row">
                <label>Voice personality</label>
                <textarea value={inst.greeting} onChange={e => updateInstance(inst.name, 'greeting', e.target.value)}
                  className="form-input-sm" rows={3}
                  placeholder="Speak warmly and casually. Use short sentences. You represent Alice's Bakery." />
              </div>
              <div className="fleet-edit-row">
                <label>Voice</label>
                <select value={inst.voice} onChange={e => updateInstance(inst.name, 'voice', e.target.value)} className="form-select-sm">
                  {VOICES.map(v => <option key={v} value={v}>{v}</option>)}
                </select>
              </div>
            </div>
          </div>
        ))}

        {/* Add unconfigured instances */}
        {instanceNames.filter(n => !configuredNames.has(n)).map(name => (
          <div key={name} className="fleet-card fleet-card-unassigned">
            <div className="fleet-card-header">
              <div className="fleet-card-name">{name}</div>
            </div>
            <div className="fleet-card-body">
              <div className="fleet-unassigned-hint">Not configured for telephony</div>
              <button className="btn btn-outline-sm" onClick={() => addInstance(name)}>Enable telephony</button>
            </div>
          </div>
        ))}
      </div>

      {/* Routing rules */}
      <h3 className="section-title" style={{ marginTop: 24 }}>Routing Rules</h3>
      <p className="page-subtitle" style={{ marginBottom: 12 }}>
        Rules are evaluated top to bottom. First match wins. Patterns: <code>default</code>, <code>ext:1001</code>, <code>did:+358*</code>, <code>caller:+1*</code>
      </p>

      <div className="routing-rules-table">
        {routes.map((rule, idx) => (
          <div key={idx} className="routing-rule-row">
            <span className="routing-rule-num">{idx + 1}</span>
            <input value={rule.pattern} onChange={e => updateRoute(idx, 'pattern', e.target.value)}
              className="form-input-sm" style={{ flex: 1 }} placeholder="default" />
            <span className="routing-arrow">→</span>
            <select value={rule.target} onChange={e => updateRoute(idx, 'target', e.target.value)}
              className="form-select-sm" style={{ flex: 1 }}>
              <option value="">-- select bot --</option>
              {instances.map(i => <option key={i.name} value={i.name}>{i.name}</option>)}
            </select>
            <button className="btn-icon" onClick={() => removeRoute(idx)} title="Remove rule">✕</button>
          </div>
        ))}
        <button className="btn btn-outline-sm" onClick={addRoute} style={{ marginTop: 8 }}>+ Add rule</button>
      </div>
    </div>
  );
}
