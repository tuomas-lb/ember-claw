import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom';
import InstanceList from './components/InstanceList';
import InstanceDetail from './components/InstanceDetail';
import DeployForm from './components/DeployForm';
import FleetEditor from './components/FleetEditor';
import CallRouting from './components/CallRouting';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 3000,
    },
  },
});

function Topbar() {
  return (
    <header className="topbar">
      <NavLink to="/" className="topbar-brand">PicoClaw Fleet</NavLink>
      <nav className="topbar-nav">
        <NavLink to="/" end className={({ isActive }) => isActive ? 'active' : ''}>
          Instances
        </NavLink>
        <NavLink to="/deploy" className={({ isActive }) => isActive ? 'active' : ''}>
          Deploy
        </NavLink>
        <NavLink to="/fleet" className={({ isActive }) => isActive ? 'active' : ''}>
          Fleet
        </NavLink>
        <NavLink to="/telephony" className={({ isActive }) => isActive ? 'active' : ''}>
          Telephony
        </NavLink>
      </nav>
    </header>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <div className="layout">
          <Topbar />
          <main className="main-content">
            <Routes>
              <Route path="/" element={<InstanceList />} />
              <Route path="/deploy" element={<DeployForm />} />
              <Route path="/instances/:name" element={<InstanceDetail />} />
              <Route path="/fleet" element={<FleetEditor />} />
              <Route path="/telephony" element={<CallRouting />} />
            </Routes>
          </main>
        </div>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
