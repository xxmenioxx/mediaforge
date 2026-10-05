import {
  Box,
  Divider,
  Drawer,
  IconButton,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Stack,
  Toolbar,
  Tooltip,
  Typography,
} from '@mui/material';
import DashboardIcon from '@mui/icons-material/Dashboard';
import FolderIcon from '@mui/icons-material/Folder';
import VideoLibraryIcon from '@mui/icons-material/VideoLibrary';
import TuneIcon from '@mui/icons-material/Tune';
import GraphicEqIcon from '@mui/icons-material/GraphicEq';
import QueueIcon from '@mui/icons-material/Queue';
import MemoryIcon from '@mui/icons-material/Memory';
import HistoryIcon from '@mui/icons-material/History';
import FactCheckIcon from '@mui/icons-material/FactCheck';
import PublishIcon from '@mui/icons-material/Publish';
import SettingsIcon from '@mui/icons-material/Settings';
import ScienceIcon from '@mui/icons-material/Science';
import AltRouteIcon from '@mui/icons-material/AltRoute';
import AssessmentOutlinedIcon from '@mui/icons-material/AssessmentOutlined';
import ArticleOutlinedIcon from '@mui/icons-material/ArticleOutlined';
import MenuIcon from '@mui/icons-material/Menu';
import CloseIcon from '@mui/icons-material/Close';
import { useState } from 'react';
import { NavLink, Outlet, useLocation } from 'react-router-dom';
import packageJson from '../../package.json';

type NavItem = {
  label: string;
  path: string;
  icon: JSX.Element;
};

type NavSection = {
  label: string;
  items: NavItem[];
};

const navSections: NavSection[] = [
  {
    label: 'Overview',
    items: [
      { label: 'Dashboard', path: '/', icon: <DashboardIcon /> },
      { label: 'Assets', path: '/assets', icon: <VideoLibraryIcon /> },
      { label: 'Reports', path: '/reports', icon: <AssessmentOutlinedIcon /> },
      { label: 'Profile Lab', path: '/profile-lab', icon: <ScienceIcon /> },
    ],
  },
  {
    label: 'Configuration',
    items: [
      { label: 'Libraries', path: '/libraries', icon: <FolderIcon /> },
      { label: 'Profiles', path: '/profiles', icon: <TuneIcon /> },
      { label: 'Audio', path: '/audio', icon: <GraphicEqIcon /> },
      { label: 'Tracks', path: '/track-profiles', icon: <AltRouteIcon /> },
    ],
  },
  {
    label: 'Pipeline',
    items: [
      { label: 'Queue', path: '/queue', icon: <QueueIcon /> },
      { label: 'Workers', path: '/workers', icon: <MemoryIcon /> },
      { label: 'Validation', path: '/validation', icon: <FactCheckIcon /> },
      { label: 'Publisher', path: '/publisher', icon: <PublishIcon /> },
    ],
  },
  {
    label: 'System',
    items: [
      { label: 'History', path: '/history', icon: <HistoryIcon /> },
      { label: 'Logs', path: '/logs', icon: <ArticleOutlinedIcon /> },
      { label: 'Settings', path: '/settings', icon: <SettingsIcon /> },
    ],
  },
];

const expandedWidth = 248;
const collapsedWidth = 68;

function isNavigationItemActive(item: NavItem, pathname: string): boolean {
  const itemPath = item.path.split('?')[0];
  if (itemPath === '/') return pathname === '/';
  return pathname === itemPath || pathname.startsWith(`${itemPath}/`);
}

function BrandMark({ size = 32 }: { size?: number }) {
  return (
    <Box
      role="img"
      aria-label="MVForge logo"
      data-testid="mvforge-brand-mark"
      sx={{
        width: size,
        height: size,
        flex: `0 0 ${size}px`,
        display: 'grid',
        placeItems: 'center',
        bgcolor: 'transparent',
      }}
    >
      <Box
        data-testid="mvforge-logo-symbol"
        aria-hidden="true"
        sx={{
          width: size * 0.78,
          height: size * 0.88,
          clipPath: 'polygon(0 0, 100% 50%, 0 100%, 0 66%, 42% 50%, 0 34%)',
          background: 'linear-gradient(145deg, #14dec0 0%, #5054ff 58%, #8b4df8 100%)',
          filter: 'drop-shadow(0 1px 4px rgba(20, 222, 192, 0.28))',
        }}
      />
    </Box>
  );
}

export function AppLayout() {
  const [isHovering, setIsHovering] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const location = useLocation();
  const isExpanded = isHovering;
  const drawerWidth = isExpanded ? expandedWidth : collapsedWidth;

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh', bgcolor: 'background.default' }}>
      <Box
        component="header"
        sx={{
          display: { xs: 'flex', md: 'none' },
          position: 'fixed',
          inset: '0 0 auto 0',
          height: 56,
          px: 1.25,
          alignItems: 'center',
          gap: 1,
          bgcolor: '#081a2b',
          borderBottom: 1,
          borderColor: '#183853',
          zIndex: (theme) => theme.zIndex.appBar,
        }}
      >
        <IconButton aria-label="Open navigation" onClick={() => setMobileOpen(true)} edge="start">
          <MenuIcon />
        </IconButton>
        <BrandMark size={30} />
        <Typography variant="h2" noWrap sx={{ fontSize: '1.05rem', letterSpacing: '0.01em' }}>MVForge</Typography>
        <Typography color="text.secondary" variant="caption" noWrap sx={{ ml: 'auto' }}>v{packageJson.version}</Typography>
      </Box>
      <Drawer
        open={mobileOpen}
        onClose={() => setMobileOpen(false)}
        variant="temporary"
        ModalProps={{ keepMounted: true }}
        sx={{ display: { xs: 'block', md: 'none' }, '& .MuiDrawer-paper': { width: 280, maxWidth: '88vw' } }}
      >
        <Stack sx={{ height: '100%' }}>
          <Toolbar sx={{ minHeight: 56, px: 1.5, gap: 1 }}>
            <BrandMark size={30} />
            <Typography variant="h2" sx={{ fontSize: '1.05rem', flex: 1 }}>MVForge</Typography>
            <IconButton aria-label="Close navigation" onClick={() => setMobileOpen(false)}><CloseIcon /></IconButton>
          </Toolbar>
          <Divider />
          <List sx={{ px: 1, py: 1, flex: 1, overflowY: 'auto' }}>
            {navSections.map((section) => (
              <Box key={section.label} sx={{ mb: 1 }}>
                <Typography color="text.secondary" variant="caption" sx={{ display: 'block', px: 1.25, py: 0.55, fontWeight: 700, fontSize: '0.68rem', letterSpacing: '0.055em', textTransform: 'uppercase' }}>
                  {section.label}
                </Typography>
                {section.items.map((item) => (
                  <ListItemButton
                    key={item.path}
                    component={NavLink}
                    to={item.path}
                    end={item.path === '/'}
                    className={isNavigationItemActive(item, location.pathname) ? 'active' : ''}
                    onClick={() => setMobileOpen(false)}
                    sx={{ borderRadius: 1, mb: 0.15, minHeight: 40, color: 'text.secondary', '&.active': { background: 'linear-gradient(90deg, rgba(108, 76, 246, 0.4), rgba(58, 41, 168, 0.75))', color: '#c3c5ff', boxShadow: 'inset 3px 0 #6c4cf6' } }}
                  >
                    <ListItemIcon sx={{ color: 'inherit', minWidth: 40 }}>{item.icon}</ListItemIcon>
                    <ListItemText primary={item.label} />
                  </ListItemButton>
                ))}
              </Box>
            ))}
          </List>
          <Divider />
          <Typography color="text.secondary" variant="caption" sx={{ p: 2 }}>Media & Video Forge · v{packageJson.version}</Typography>
        </Stack>
      </Drawer>
      <Box
        component="aside"
        onMouseEnter={() => setIsHovering(true)}
        onMouseLeave={() => setIsHovering(false)}
        sx={{
          width: drawerWidth,
          position: { md: 'fixed' },
          top: 0,
          bottom: 0,
          left: 0,
          zIndex: (theme) => theme.zIndex.drawer + 10,
          borderRight: 1,
          borderColor: '#183853',
          bgcolor: '#081a2b',
          display: { xs: 'none', md: 'block' },
          overflowX: 'hidden',
          transition: 'width 160ms ease',
        }}
      >
        <Stack sx={{ minHeight: '100%' }}>
          <Toolbar
            sx={{
              minHeight: 64,
              px: isExpanded ? 1.5 : 0.75,
              gap: 1,
              justifyContent: isExpanded ? 'flex-start' : 'center',
            }}
          >
            {isExpanded ? <BrandMark size={32} /> : null}
            {isExpanded ? (
              <Stack spacing={0} sx={{ minWidth: 0 }}>
                <Typography variant="h2" noWrap sx={{ fontSize: '1rem', lineHeight: 1.2, letterSpacing: '0.01em' }}>MVForge</Typography>
                <Typography color="text.secondary" variant="caption" noWrap sx={{ fontSize: '0.66rem' }}>MEDIA WORKSPACE</Typography>
              </Stack>
            ) : (
              <Tooltip title="MVForge" placement="right"><Box><BrandMark size={34} /></Box></Tooltip>
            )}
          </Toolbar>
          <Divider />
          <List sx={{ px: 0.75, py: 1, flex: 1, overflowY: 'auto' }}>
            {navSections.map((section) => (
              <Box key={section.label} sx={{ mb: isExpanded ? 1.1 : 0.4 }}>
                {isExpanded ? (
                  <Typography
                    color="text.secondary"
                    variant="caption"
                    sx={{ display: 'block', px: 1.25, py: 0.55, fontWeight: 700, fontSize: '0.66rem', letterSpacing: '0.055em', textTransform: 'uppercase' }}
                  >
                    {section.label}
                  </Typography>
                ) : null}
                {section.items.map((item) => {
                  const button = (
                    <ListItemButton
                      key={item.path}
                      component={NavLink}
                      to={item.path}
                      end={item.path === '/'}
                      className={isNavigationItemActive(item, location.pathname) ? 'active' : ''}
                      sx={{
                        borderRadius: 1,
                        mb: 0.2,
                        minHeight: 40,
                        justifyContent: isExpanded ? 'flex-start' : 'center',
                        color: 'text.secondary',
                        px: isExpanded ? 1.15 : 0.75,
                        '&.active': {
                          background: 'linear-gradient(90deg, rgba(108, 76, 246, 0.4), rgba(58, 41, 168, 0.75))',
                          color: '#c3c5ff',
                          boxShadow: 'inset 3px 0 #6c4cf6',
                        },
                        '&:hover': { bgcolor: 'rgba(17, 42, 68, 0.72)', color: '#e7eef8' },
                      }}
                    >
                      <ListItemIcon
                        sx={{
                          color: 'inherit',
                          minWidth: isExpanded ? 38 : 0,
                          justifyContent: 'center',
                        }}
                      >
                        {item.icon}
                      </ListItemIcon>
                      {isExpanded ? <ListItemText primary={item.label} primaryTypographyProps={{ noWrap: true }} /> : null}
                    </ListItemButton>
                  );

                  return isExpanded ? (
                    button
                  ) : (
                    <Tooltip key={item.path} title={item.label} placement="right">
                      {button}
                    </Tooltip>
                  );
                })}
              </Box>
            ))}
          </List>
          <Divider />
          <Box sx={{ px: isExpanded ? 2 : 1, py: 1.25, textAlign: isExpanded ? 'left' : 'center' }}>
            <Typography color="text.secondary" variant="caption" noWrap sx={{ fontSize: '0.64rem' }}>
              {isExpanded ? `MVForge · Media & Video Forge · v${packageJson.version}` : `v${packageJson.version}`}
            </Typography>
          </Box>
        </Stack>
      </Box>
      <Box component="main" sx={{ flex: 1, minWidth: 0, ml: { md: `${collapsedWidth}px` }, pt: { xs: '56px', md: 0 }, maxWidth: '100%' }}>
        <Outlet />
      </Box>
    </Box>
  );
}
