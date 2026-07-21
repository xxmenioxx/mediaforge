import { Navigate, Route, Routes } from 'react-router-dom';
import { AppLayout } from './components/AppLayout';
import { DashboardPage } from './pages/DashboardPage';
import { LibrariesPage } from './pages/LibrariesPage';
import { AssetsPage } from './pages/AssetsPage';
import { AnalysisPage } from './pages/AnalysisPage';
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
        <Route path="analysis" element={<AnalysisPage />} />
        <Route path="profile-lab" element={<ProfileLabPage />} />
        <Route path="profiles" element={<ProfilesPage />} />
        <Route path="audio" element={<AudioProfilesPage />} />
        <Route path="track-profiles" element={<TrackProfilesPage />} />
        <Route path="scanner" element={<Navigate to="/analysis" replace />} />
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
