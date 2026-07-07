import { createContext, useContext, useState, useCallback, useEffect } from 'react';

type ToastType = 'success' | 'error' | 'info';

interface ToastItem {
  id: number;
  type: ToastType;
  message: string;
}

interface ToastContextValue {
  showToast: (message: string, type?: ToastType) => void;
}

const ToastContext = createContext<ToastContextValue>({ showToast: () => {} });

let _id = 0;

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const showToast = useCallback((message: string, type: ToastType = 'info') => {
    const id = ++_id;
    setToasts(prev => [...prev, { id, type, message }]);
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, 3500);
  }, []);

  return (
    <ToastContext.Provider value={{ showToast }}>
      {children}
      <div className="toast-container">
        {toasts.map(t => (
          <div key={t.id} className={`toast toast-${t.type}`}>
            {t.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  return useContext(ToastContext);
}

// Standalone hook: wrap at app level if needed, or just import showToast directly.
// This simple version re-exports a module-level emitter.

type Listener = (msg: string, type: ToastType) => void;
const listeners: Listener[] = [];

export function emitToast(message: string, type: ToastType = 'info') {
  listeners.forEach(l => l(message, type));
}

export function useToastEmitter() {
  const { showToast } = useContext(ToastContext);
  useEffect(() => {
    listeners.push(showToast);
    return () => {
      const idx = listeners.indexOf(showToast);
      if (idx !== -1) listeners.splice(idx, 1);
    };
  }, [showToast]);
}
