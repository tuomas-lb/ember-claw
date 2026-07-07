import { useEffect, useState } from 'react';
import { getConfig, pushConfig } from '../api/client';

interface Props {
  instanceName: string;
}

type SaveState = 'idle' | 'saving' | 'success' | 'error';

export default function ConfigEditor({ instanceName }: Props) {
  const [raw, setRaw] = useState('');
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveState, setSaveState] = useState<SaveState>('idle');
  const [saveError, setSaveError] = useState<string | null>(null);
  const [parseError, setParseError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    setLoadError(null);
    getConfig(instanceName)
      .then(cfg => {
        setRaw(JSON.stringify(cfg, null, 2));
      })
      .catch((e: Error) => setLoadError(e.message))
      .finally(() => setLoading(false));
  }, [instanceName]);

  function handleChange(value: string) {
    setRaw(value);
    setParseError(null);
    try {
      JSON.parse(value);
    } catch (e) {
      setParseError((e as Error).message);
    }
  }

  async function save() {
    setParseError(null);
    setSaveError(null);
    let parsed: unknown;
    try {
      parsed = JSON.parse(raw);
    } catch (e) {
      setParseError((e as Error).message);
      return;
    }
    setSaveState('saving');
    try {
      await pushConfig(instanceName, parsed);
      setSaveState('success');
      setTimeout(() => setSaveState('idle'), 2000);
    } catch (e) {
      setSaveState('error');
      setSaveError((e as Error).message);
    }
  }

  function format() {
    try {
      const parsed = JSON.parse(raw);
      setRaw(JSON.stringify(parsed, null, 2));
      setParseError(null);
    } catch (e) {
      setParseError((e as Error).message);
    }
  }

  if (loading) return <div className="loading">loading config...</div>;
  if (loadError) return <div className="error-box">Failed to load config: {loadError}</div>;

  return (
    <div className="config-editor-wrap">
      <div className="config-actions">
        <button
          className="btn btn-primary"
          onClick={save}
          disabled={saveState === 'saving' || !!parseError}
        >
          {saveState === 'saving' ? 'saving...' : saveState === 'success' ? 'saved!' : 'save'}
        </button>
        <button className="btn btn-outline" onClick={format} disabled={!!parseError}>
          format
        </button>
        {saveState === 'success' && (
          <span className="text-green" style={{ fontSize: 12 }}>config saved</span>
        )}
        {saveState === 'error' && saveError && (
          <span className="text-red" style={{ fontSize: 12 }}>{saveError}</span>
        )}
      </div>

      <textarea
        className="form-textarea"
        value={raw}
        onChange={e => handleChange(e.target.value)}
        spellCheck={false}
        style={{
          minHeight: 400,
          fontSize: 12,
          lineHeight: 1.6,
          fontFamily: 'var(--font-mono)',
          background: '#050505',
          color: '#c8e0c8',
          border: parseError ? '1px solid var(--red)' : '1px solid var(--border)',
        }}
      />

      {parseError && (
        <div className="form-error">JSON parse error: {parseError}</div>
      )}
    </div>
  );
}
