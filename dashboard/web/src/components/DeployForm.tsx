import { useState, useEffect, FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { deployInstance, DeployRequest, listModels, ProviderModel } from '../api/client';

const PROVIDERS = [
  { value: 'anthropic',   label: 'Anthropic' },
  { value: 'openai',      label: 'OpenAI' },
  { value: 'gemini',      label: 'Google Gemini' },
  { value: 'deepseek',    label: 'DeepSeek' },
  { value: 'mistral',     label: 'Mistral' },
  { value: 'xai',         label: 'xAI' },
  { value: 'kimi',        label: 'Moonshot / Kimi' },
  { value: 'groq',        label: 'Groq' },
  { value: 'openrouter',  label: 'OpenRouter' },
  { value: 'copilot',     label: 'GitHub Copilot' },
];

const DEFAULT_MODELS: Record<string, string> = {
  anthropic:  'claude-sonnet-4-5-20250514',
  openai:     'gpt-4o-mini',
  gemini:     'gemini-2.5-flash-preview-05-20',
  deepseek:   'deepseek-chat',
  mistral:    'codestral-latest',
  xai:        'grok-3',
  kimi:       'moonshot-v1-auto',
  groq:       'llama-3.1-8b-instant',
  openrouter: 'openai/gpt-4o-mini',
  copilot:    'gpt-4o',
};

const DNS_RE = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/;

function validate(form: typeof EMPTY_FORM): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!form.name) {
    errors.name = 'required';
  } else if (!DNS_RE.test(form.name)) {
    errors.name = 'must be lowercase alphanumeric + hyphens';
  } else if (form.name.length > 40) {
    errors.name = 'max 40 characters';
  }
  if (!form.provider) errors.provider = 'required';
  if (!form.model)    errors.model    = 'required';
  if (!form.api_key)  errors.api_key  = 'required';
  return errors;
}

const EMPTY_FORM = {
  name:         '',
  provider:     'anthropic',
  model:        DEFAULT_MODELS['anthropic'],
  api_key:      '',
  cpu_limit:    '',
  memory_limit: '',
  storage_size: '',
};

export default function DeployForm() {
  const navigate = useNavigate();
  const [form, setForm] = useState(EMPTY_FORM);
  const [touched, setTouched] = useState<Record<string, boolean>>({});
  const [showAdvanced, setShowAdvanced] = useState(false);

  // Dynamic model fetching
  const [models, setModels] = useState<ProviderModel[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelsError, setModelsError] = useState('');

  const errors = validate(form);
  const hasErrors = Object.keys(errors).length > 0;

  // Fetch models when provider + api_key are both set
  useEffect(() => {
    if (!form.provider || !form.api_key || form.api_key.length < 10) {
      setModels([]);
      return;
    }

    const timeout = setTimeout(async () => {
      setModelsLoading(true);
      setModelsError('');
      try {
        const result = await listModels(form.provider, form.api_key);
        setModels(result);
      } catch (err: any) {
        setModelsError(err.message || 'Failed to fetch models');
        setModels([]);
      } finally {
        setModelsLoading(false);
      }
    }, 500); // debounce

    return () => clearTimeout(timeout);
  }, [form.provider, form.api_key]);

  const deployMut = useMutation({
    mutationFn: (req: DeployRequest) => deployInstance(req),
    onSuccess: () => navigate(`/instances/${form.name}`),
  });

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setTouched({ name: true, provider: true, model: true, api_key: true });
    if (hasErrors) return;

    const req: DeployRequest = {
      name:     form.name,
      provider: form.provider,
      model:    form.model,
      api_key:  form.api_key,
    };
    if (form.cpu_limit)    req.cpu_limit    = form.cpu_limit;
    if (form.memory_limit) req.memory_limit = form.memory_limit;
    if (form.storage_size) req.storage_size = form.storage_size;

    deployMut.mutate(req);
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Deploy Instance</div>
          <div className="page-subtitle">Create a new PicoClaw AI assistant instance</div>
        </div>
      </div>

      <form className="deploy-form" onSubmit={handleSubmit} noValidate>
        <div className="card">
          <div className="form-group">
            <label className="form-label" htmlFor="name">Instance Name</label>
            <input
              id="name" type="text" placeholder="my-assistant"
              value={form.name}
              onChange={e => setForm(p => ({ ...p, name: e.target.value }))}
              onBlur={() => setTouched(p => ({ ...p, name: true }))}
              className={`form-input${touched.name && errors.name ? ' invalid' : ''}`}
            />
            {touched.name && errors.name && <div className="form-error">{errors.name}</div>}
            <div className="form-hint">DNS-safe: lowercase letters, numbers, hyphens</div>
          </div>

          <div className="form-group">
            <label className="form-label" htmlFor="provider">Provider</label>
            <select
              id="provider" value={form.provider}
              onChange={e => {
                const val = e.target.value;
                setForm(p => ({ ...p, provider: val, model: DEFAULT_MODELS[val] ?? '' }));
                setModels([]);
              }}
              className="form-select"
            >
              {PROVIDERS.map(p => (
                <option key={p.value} value={p.value}>{p.label}</option>
              ))}
            </select>
          </div>

          <div className="form-group">
            <label className="form-label" htmlFor="api_key">API Key</label>
            <input
              id="api_key" type="password" placeholder="sk-..." autoComplete="off"
              value={form.api_key}
              onChange={e => setForm(p => ({ ...p, api_key: e.target.value }))}
              onBlur={() => setTouched(p => ({ ...p, api_key: true }))}
              className={`form-input${touched.api_key && errors.api_key ? ' invalid' : ''}`}
            />
            {touched.api_key && errors.api_key && <div className="form-error">{errors.api_key}</div>}
            {modelsLoading && <div className="form-hint" style={{ color: '#00ff88' }}>Fetching models...</div>}
            {modelsError && <div className="form-hint" style={{ color: '#ff6b35' }}>{modelsError}</div>}
          </div>

          <div className="form-group">
            <label className="form-label" htmlFor="model">Model</label>
            {models.length > 0 ? (
              <select
                id="model" value={form.model}
                onChange={e => setForm(p => ({ ...p, model: e.target.value }))}
                className="form-select"
              >
                <option value="">-- select model --</option>
                {models.map(m => (
                  <option key={m.id} value={m.id}>
                    {m.display_name !== m.id ? `${m.display_name} (${m.id})` : m.id}
                  </option>
                ))}
              </select>
            ) : (
              <input
                id="model" type="text" placeholder="model-name"
                value={form.model}
                onChange={e => setForm(p => ({ ...p, model: e.target.value }))}
                onBlur={() => setTouched(p => ({ ...p, model: true }))}
                className={`form-input${touched.model && errors.model ? ' invalid' : ''}`}
              />
            )}
            {touched.model && errors.model && <div className="form-error">{errors.model}</div>}
            {models.length > 0 && (
              <div className="form-hint">{models.length} models available</div>
            )}
            {!models.length && !modelsLoading && form.api_key.length >= 10 && (
              <div className="form-hint">Enter API key to load available models, or type manually</div>
            )}
          </div>

          <hr className="section-divider" />

          <button type="button" className="collapsible-header" onClick={() => setShowAdvanced(p => !p)}>
            <span className={`collapsible-arrow ${showAdvanced ? 'open' : ''}`}>&#9654;</span>
            Advanced Options
          </button>

          {showAdvanced && (
            <div className="mt-12">
              <div className="form-row">
                <div className="form-group">
                  <label className="form-label" htmlFor="cpu_limit">CPU Limit</label>
                  <input id="cpu_limit" type="text" placeholder="1000m"
                    value={form.cpu_limit} onChange={e => setForm(p => ({ ...p, cpu_limit: e.target.value }))}
                    className="form-input" />
                  <div className="form-hint">e.g. 500m, 1, 2</div>
                </div>
                <div className="form-group">
                  <label className="form-label" htmlFor="memory_limit">Memory Limit</label>
                  <input id="memory_limit" type="text" placeholder="1Gi"
                    value={form.memory_limit} onChange={e => setForm(p => ({ ...p, memory_limit: e.target.value }))}
                    className="form-input" />
                  <div className="form-hint">e.g. 256Mi, 512Mi, 1Gi</div>
                </div>
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="storage_size">Storage Size</label>
                <input id="storage_size" type="text" placeholder="5Gi"
                  value={form.storage_size} onChange={e => setForm(p => ({ ...p, storage_size: e.target.value }))}
                  className="form-input" style={{ maxWidth: 200 }} />
                <div className="form-hint">PVC size, e.g. 1Gi, 5Gi</div>
              </div>
            </div>
          )}

          <div className="mt-16" style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
            <button type="submit" className="btn btn-primary" disabled={deployMut.isPending}>
              {deployMut.isPending ? 'deploying...' : 'Deploy Instance'}
            </button>
            <button type="button" className="btn btn-outline" onClick={() => navigate('/')}>cancel</button>
          </div>

          {deployMut.isError && (
            <div className="error-box mt-12">
              Deploy failed: {(deployMut.error as Error).message}
            </div>
          )}
        </div>
      </form>
    </div>
  );
}
