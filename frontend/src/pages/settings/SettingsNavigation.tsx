import {
  Box,
  Chip,
  FormControl,
  InputLabel,
  List,
  ListItemButton,
  ListItemText,
  MenuItem,
  Paper,
  Select,
  Stack,
  Typography,
} from '@mui/material';
import { settingsDraftSectionForRoute } from './settingsDrafts';

export const settingsSections = [
  'general',
  'pipeline',
  'analysis',
  'runtime',
  'assets',
  'asset-defaults',
  'validation',
  'providers-integrations',
  'archive',
  'operations',
  'subtitles',
  'test-encode',
  'advanced',
] as const;

export type SettingsSection = (typeof settingsSections)[number];

export type SettingsDraftSection = Extract<
  SettingsSection,
  'general' | 'pipeline' | 'analysis' | 'runtime' | 'assets' | 'asset-defaults' | 'validation' | 'archive' | 'operations' | 'subtitles' | 'test-encode' | 'advanced'
>;

type SettingsSectionDefinition = {
  key: SettingsSection;
  title: string;
  description: string;
  note?: string;
};

export const settingsSectionDefinitions: SettingsSectionDefinition[] = [
  {
    key: 'general',
    title: 'General',
    description: 'Set shared user preferences such as preferred quality, encoder, and languages.',
    note: 'These preferences are fallbacks for future work; frozen Queue jobs keep their saved execution settings.',
  },
  {
    key: 'pipeline',
    title: 'Pipeline & Scheduling',
    description: 'Control workflow automation and the hours when scheduled work may start.',
    note: 'This section controls workflow policy. Worker capacity, runtime capability, and executor definitions live under Runtime & Executors.',
  },
  {
    key: 'analysis',
    title: 'Analysis',
    description: 'Choose how MVForge samples media, evaluates confidence, and reuses analysis snapshots.',
    note: 'Analysis policy affects future analysis runs; existing frozen Queue executions keep their captured inputs.',
  },
  {
    key: 'runtime',
    title: 'Runtime & Executors',
    description: 'Set worker capacity, choose runtime profiles, configure external executors, and inspect playback compatibility and host capabilities.',
    note: 'Runtime refresh and executor probes are immediate actions. Worker capacity and runtime changes do not cancel running jobs.',
  },
  {
    key: 'assets',
    title: 'Assets & Storage',
    description: 'Manage asset classification, inventory, controlled paths, storage roles, and workspace policy.',
    note: 'Asset classification, storage locations, and Library destinations are separate settings. Inventory policy applies to future reconciliation runs.',
  },
  {
    key: 'asset-defaults',
    title: 'Asset Defaults',
    description: 'Set fallback video, audio, track, subtitle, and attachment behavior for assets.',
    note: 'These are processing defaults for future preparation; they do not rewrite frozen Queue jobs or Asset overrides.',
  },
  {
    key: 'validation',
    title: 'Validation & Publishing',
    description: 'Set quality checks, automatic advancement, and publication behavior for completed jobs.',
    note: 'Policy changes govern future validation and publication decisions. Already frozen executions retain their captured configuration.',
  },
  {
    key: 'providers-integrations',
    title: 'Providers & Integrations',
    description: 'Connect media servers and provider services using protected credential settings.',
    note: 'Connection tests and discovery run immediately; saving a connection does not edit frozen Queue executions.',
  },
  {
    key: 'archive',
    title: 'Archive & Retention',
    description: 'Choose where originals are archived and how long processed originals are retained.',
    note: 'Retention policy applies to future cleanup decisions. It does not change the identity or contents of existing media.',
  },
  {
    key: 'operations',
    title: 'Operations & Maintenance',
    description: 'Run scheduler recovery, preview housekeeping, and open logs or version diagnostics.',
    note: 'Actions in this section execute immediately. They are separate from configuration saves.',
  },
  {
    key: 'subtitles',
    title: 'Subtitles',
    description: 'Configure subtitle styling and text compatibility behavior.',
    note: 'Subtitle defaults affect future processing; existing frozen Queue jobs keep their captured subtitle plan.',
  },
  {
    key: 'test-encode',
    title: 'Test Encode',
    description: 'Set test encode defaults and review retained test runs and artifacts.',
    note: 'Test encode settings apply to future test runs only.',
  },
  {
    key: 'advanced',
    title: 'Advanced',
    description: 'Inspect or edit the raw JSON representation of core settings for expert workflows.',
    note: 'This editor owns paths, workers, pipeline automation, inventory, cancellation, archive retention, asset types, and categories. It overrides matching standard sections when saved; frozen Queue jobs keep their captured configuration.',
  },
];

export function resolveSettingsSection(value: string | null): SettingsSection {
  if (value === 'overview') {
    return 'general';
  }

  return settingsSections.includes(value as SettingsSection)
    ? (value as SettingsSection)
    : 'general';
}

export function settingsSectionForDraft(value: SettingsSection): SettingsDraftSection | undefined {
  return settingsDraftSectionForRoute(value);
}

export function SettingsSidebar({
  section,
  dirtySections,
  onChange,
}: {
  section: SettingsSection;
  dirtySections: SettingsDraftSection[];
  onChange: (section: SettingsSection) => void;
}) {
  const hasDraft = (key: SettingsSection) => {
    const draftSection = settingsSectionForDraft(key);
    return draftSection ? dirtySections.includes(draftSection) : false;
  };

  return (
    <Box component="nav" aria-label="Settings sections">
      <FormControl fullWidth sx={{ display: { xs: 'block', md: 'none' }, mb: 2 }}>
        <InputLabel id="settings-section-label">Settings section</InputLabel>
        <Select
          labelId="settings-section-label"
          label="Settings section"
          value={section}
          onChange={(event) => onChange(event.target.value as SettingsSection)}
        >
          {settingsSectionDefinitions.map((item) => (
            <MenuItem key={item.key} value={item.key}>
              {item.title}{hasDraft(item.key) ? ' · Unsaved changes' : ''}
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <Paper
        variant="outlined"
        sx={{
          display: { xs: 'none', md: 'block' },
          position: 'sticky',
          top: 16,
          maxHeight: 'calc(100vh - 112px)',
          overflowY: 'auto',
          p: 1,
        }}
      >
        <Stack spacing={1} sx={{ px: 1, pt: 1 }}>
          <Typography variant="h3">Settings</Typography>
          <Typography color="text.secondary" variant="body2">
            Choose a domain to view its controls.
          </Typography>
        </Stack>
        <List aria-label="Settings domains" disablePadding sx={{ mt: 1 }}>
          {settingsSectionDefinitions.map((item) => (
            <ListItemButton
              key={item.key}
              selected={section === item.key}
              aria-current={section === item.key ? 'page' : undefined}
              onClick={() => onChange(item.key)}
              sx={{ borderRadius: 1, alignItems: 'flex-start' }}
            >
              <ListItemText
                primary={item.title}
                secondary={hasDraft(item.key) ? 'Unsaved changes' : undefined}
                primaryTypographyProps={{ variant: 'body2', fontWeight: section === item.key ? 700 : 500 }}
              />
              {hasDraft(item.key) ? <Chip size="small" label="Draft" color="warning" sx={{ ml: 1 }} /> : null}
            </ListItemButton>
          ))}
        </List>
      </Paper>
    </Box>
  );
}

export function SettingsSectionHeader({
  section,
  saveState,
}: {
  section: SettingsSection;
  saveState?: { label: string; color?: 'default' | 'success' | 'warning' | 'error' };
}) {
  const definition = settingsSectionDefinitions.find((item) => item.key === section)!;

  return (
    <Stack component="header" spacing={1} sx={{ mb: 2 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" spacing={1}>
        <Stack spacing={0.5}>
          <Typography variant="h2">{definition.title}</Typography>
          <Typography color="text.secondary">{definition.description}</Typography>
        </Stack>
        {saveState ? <Chip label={saveState.label} color={saveState.color ?? 'default'} size="small" /> : null}
      </Stack>
      {definition.note ? <Typography variant="body2" color="text.secondary">{definition.note}</Typography> : null}
    </Stack>
  );
}
