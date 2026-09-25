import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { AppLayout } from './components/AppLayout';
import { DashboardPage } from './pages/DashboardPage';
import { LibrariesPage } from './pages/LibrariesPage';
import { AssetsPage } from './pages/AssetsPage';
import { AudioProfilesPage } from './pages/AudioProfilesPage';
import { ProfileLabPage } from './pages/ProfileLabPage';
import { ProfilesPage } from './pages/ProfilesPage';
import { QueuePage } from './pages/QueuePage';
import { WorkersPage } from './pages/WorkersPage';
import { ValidationPage } from './pages/ValidationPage';
import { PublisherPage } from './pages/PublisherPage';
import { LogsPage } from './pages/LogsPage';
import { SettingsPage } from './pages/SettingsPage';
import { VersionsPage } from './pages/VersionsPage';
import { HistoryPage } from './pages/HistoryPage';
import { TrackProfilesPage } from './pages/TrackProfilesPage';

export default function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<DashboardPage />} />
        <Route path="libraries" element={<LibrariesPage />} />
        <Route path="assets" element={<AssetsPage />} />
        <Route path="assets/:legacyView" element={<LegacyAssetsRoute />} />
        <Route path="analysis" element={<LegacyAnalysisRoute />} />
        <Route path="profile-lab" element={<ProfileLabPage />} />
        <Route path="profiles" element={<ProfilesPage />} />
        <Route path="audio" element={<AudioProfilesPage />} />
        <Route path="track-profiles" element={<TrackProfilesPage />} />
        <Route path="scanner" element={<LegacyAnalysisRoute />} />
        <Route path="queue" element={<QueuePage />} />
        <Route path="workers" element={<WorkersPage />} />
        <Route path="history" element={<HistoryPage />} />
        <Route path="validation" element={<ValidationPage />} />
        <Route path="publisher" element={<PublisherPage />} />
        <Route path="logs" element={<LogsPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="versions" element={<VersionsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}

export function LegacyAnalysisRoute() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  const asset = params.get('asset') ?? params.get('assetPath') ?? params.get('path');
  params.delete('autorun');
  params.delete('assetPath');
  params.delete('path');
  params.set('location', 'raw');
  if (asset) params.set('asset', asset);
  return <Navigate to={`/assets?${params.toString()}`} replace />;
}

export function LegacyAssetsRoute() {
  const location = useLocation();
  const view = location.pathname.split('/').filter(Boolean).at(-1);
  const params = new URLSearchParams(location.search);
  params.delete('tab');
  if (view === 'unprocessed') {
    params.set('location', 'raw');
  } else if (view === 'converted') {
    params.set('location', 'library');
    params.set('provenance', 'converted');
  } else if (view === 'archive') {
    params.set('location', 'library');
    params.set('original', 'archived');
  } else if (view === 'reports') {
    params.set('location', 'library');
    params.set('evidence', 'reports');
  } else {
    return <div role="alert">This Assets route is no longer an inventory context. Open RAW or Library and use the persisted lifecycle filters.</div>;
  }
  return <Navigate to={`/assets${params.toString() ? `?${params.toString()}` : ''}`} replace />;
}
