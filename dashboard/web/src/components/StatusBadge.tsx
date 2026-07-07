interface Props {
  status: string;
  ready?: boolean;
}

function badgeClass(status: string): string {
  const s = status.toLowerCase();
  if (s === 'running') return 'badge-running';
  if (s === 'pending' || s === 'containercreating' || s === 'init') return 'badge-pending';
  if (
    s.includes('error') ||
    s.includes('crash') ||
    s === 'oomkilled' ||
    s === 'failed' ||
    s === 'evicted'
  ) return 'badge-error';
  return 'badge-unknown';
}

export default function StatusBadge({ status }: Props) {
  return (
    <span className={`badge ${badgeClass(status)}`}>
      {status}
    </span>
  );
}
