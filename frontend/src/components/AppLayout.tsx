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
import TroubleshootIcon from '@mui/icons-material/Troubleshoot';
import AltRouteIcon from '@mui/icons-material/AltRoute';
import MenuIcon from '@mui/icons-material/Menu';
import CloseIcon from '@mui/icons-material/Close';
import { useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
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
      { label: 'Analysis', path: '/analysis', icon: <TroubleshootIcon /> },
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
      { label: 'Settings', path: '/settings', icon: <SettingsIcon /> },
    ],
  },
];

const expandedWidth = 248;
const collapsedWidth = 76;

export function AppLayout() {
  const [isHovering, setIsHovering] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
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
          bgcolor: 'background.paper',
          borderBottom: 1,
          borderColor: 'divider',
          zIndex: (theme) => theme.zIndex.appBar,
        }}
      >
        <IconButton aria-label="Open navigation" onClick={() => setMobileOpen(true)} edge="start">
          <MenuIcon />
        </IconButton>
        <Typography variant="h2" noWrap sx={{ fontSize: '1.15rem' }}>MVForge</Typography>
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
          <Toolbar sx={{ minHeight: 56, px: 2, gap: 1 }}>
            <Typography variant="h2" sx={{ fontSize: '1.15rem', flex: 1 }}>MVForge</Typography>
            <IconButton aria-label="Close navigation" onClick={() => setMobileOpen(false)}><CloseIcon /></IconButton>
          </Toolbar>
          <Divider />
          <List sx={{ px: 1, py: 1, flex: 1, overflowY: 'auto' }}>
            {navSections.map((section) => (
              <Box key={section.label} sx={{ mb: 1 }}>
                <Typography color="text.secondary" variant="caption" sx={{ display: 'block', px: 1.5, py: 0.75, fontWeight: 700, textTransform: 'uppercase' }}>
                  {section.label}
                </Typography>
                {section.items.map((item) => (
                  <ListItemButton
                    key={item.path}
                    component={NavLink}
                    to={item.path}
                    end={item.path === '/'}
                    onClick={() => setMobileOpen(false)}
                    sx={{ borderRadius: 1, mb: 0.25, minHeight: 44, color: 'text.secondary', '&.active': { bgcolor: 'rgba(79,179,255,0.14)', color: 'primary.main' } }}
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
          borderColor: 'divider',
          bgcolor: 'background.paper',
          display: { xs: 'none', md: 'block' },
          overflowX: 'hidden',
          transition: 'width 160ms ease',
        }}
      >
        <Stack sx={{ minHeight: '100%' }}>
          <Toolbar
            sx={{
              minHeight: 72,
              px: isExpanded ? 2 : 1,
              justifyContent: isExpanded ? 'space-between' : 'center',
            }}
          >
            {isExpanded ? (
              <Stack spacing={0.2} sx={{ minWidth: 0 }}>
                <Typography variant="h2" noWrap>
                  MVForge
                </Typography>
                <Typography color="text.secondary" variant="body2" noWrap>
                  Manual media workflows
                </Typography>
              </Stack>
            ) : null}
            {!isExpanded ? <Typography variant="h2">MVF</Typography> : null}
          </Toolbar>
          <Divider />
          <List sx={{ px: 1, py: 1.5, flex: 1, overflowY: 'auto' }}>
            {navSections.map((section) => (
              <Box key={section.label} sx={{ mb: isExpanded ? 1.5 : 0.5 }}>
                {isExpanded ? (
                  <Typography
                    color="text.secondary"
                    variant="caption"
                    sx={{ display: 'block', px: 1.25, py: 0.75, fontWeight: 700, textTransform: 'uppercase' }}
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
                      sx={{
                        borderRadius: 1,
                        mb: 0.35,
                        minHeight: 44,
                        justifyContent: isExpanded ? 'flex-start' : 'center',
                        color: 'text.secondary',
                        px: isExpanded ? 1.25 : 1,
                        '&.active': {
                          bgcolor: 'rgba(79,179,255,0.14)',
                          color: 'primary.main',
                        },
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
            <Typography color="text.secondary" variant="caption" noWrap>
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
