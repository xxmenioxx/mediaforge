import type { Dispatch, SetStateAction } from 'react';
import { Alert, Autocomplete, Button, Card, CardContent, Chip, FormControlLabel, Grid, IconButton, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Tooltip, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import type { AppSetting, Library } from '../../api/types';
import { LegacyAnalysisRecords } from '../../components/LegacyAnalysisRecords';
import { useSettingsDraftValue } from '../../hooks/useSettingsDraftValue';
import { booleanValue, buildSettingsForm, defaultAssetCategories, normalizeStringArray, storageRoleNames, storageRolesValue } from '../../utils/settingsPageHelpers';
import type { LibraryType, SettingsForm, StorageRolesValue, WorkspaceValue } from '../SettingsPage';

type SettingsCardProps = { form: SettingsForm; setForm: Dispatch<SetStateAction<SettingsForm>> };
function identitySettingsValue<T>(value: unknown): T { return value as T; }

export function AssetCategoriesPanel({ categories, onChange }: { categories: string[]; onChange: (categories: string[]) => void }) {
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Asset Categories</Typography>
            <Typography color="text.secondary" variant="body2">
              Define controlled labels for classifying assets and groups. These are separate from library destination types.
            </Typography>
          </Stack>
          <Autocomplete
            multiple
            freeSolo
            options={defaultAssetCategories}
            value={categories}
            onChange={(_, value) => onChange(value.map((category) => category.trim()).filter(Boolean))}
            renderTags={(value, getTagProps) =>
              value.map((option, index) => {
                const { key, ...tagProps } = getTagProps({ index });
                return <Chip key={key} label={option} size="small" {...tagProps} />;
              })
            }
            renderInput={(params) => (
              <TextField {...params} label="Controlled asset categories" placeholder="Add category" helperText="Examples: anime, movie, extras, episode, season." />
            )}
            fullWidth
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

export function LibraryTypesPanel({
  assetTypes,
  librariesByType,
  onAdd,
  onEdit,
  onDelete,
}: {
  assetTypes: LibraryType[];
  librariesByType: Record<string, number>;
  onAdd: () => void;
  onEdit: (type: LibraryType) => void;
  onDelete: (type: LibraryType) => void;
}) {
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
            <Stack>
              <Typography variant="h3">Library Types</Typography>
              <Typography color="text.secondary" variant="body2">
                Define the friendly categories used when creating libraries.
              </Typography>
            </Stack>
            <Button startIcon={<AddIcon />} variant="contained" onClick={onAdd}>
              Add Type
            </Button>
          </Stack>
          <Table sx={{ tableLayout: 'fixed' }}>
            <TableHead>
              <TableRow>
                <TableCell>Type</TableCell>
                <TableCell>Extensions</TableCell>
                <TableCell>Libraries</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {assetTypes.map((type) => (
                <TableRow key={type.key} hover>
                  <TableCell>
                    <Stack spacing={0.4}>
                      <Typography fontWeight={700}>{type.label}</Typography>
                      <Typography color="text.secondary" variant="body2">
                        {type.key}
                      </Typography>
                      {type.description ? (
                        <Typography color="text.secondary" variant="body2">
                          {type.description}
                        </Typography>
                      ) : null}
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                      {type.extensions.map((extension) => (
                        <Chip key={extension} label={extension} size="small" />
                      ))}
                    </Stack>
                  </TableCell>
                  <TableCell>{librariesByType[type.key] ?? 0}</TableCell>
                  <TableCell align="right">
                    <Stack direction="row" spacing={1} justifyContent="flex-end">
                      <Tooltip title="Edit type">
                        <IconButton onClick={() => onEdit(type)} aria-label={`Edit ${type.label}`}>
                          <EditIcon />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Delete type">
                        <IconButton color="error" onClick={() => onDelete(type)} aria-label={`Delete ${type.label}`}>
                          <DeleteIcon />
                        </IconButton>
                      </Tooltip>
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function PathsCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Paths</Typography>
            <Typography color="text.secondary" variant="body2">
              Global source root and controlled output locations.
            </Typography>
          </Stack>
          <TextField
            label="Raw root"
            value={form.paths.rawRoot}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, rawRoot: event.target.value } }))
            }
            fullWidth
          />
          <TextField
            label="Library root"
            value={form.paths.libraryRoot}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, libraryRoot: event.target.value } }))
            }
            fullWidth
          />
          <TextField
            label="Staging path"
            value={form.paths.stagingPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, stagingPath: event.target.value } }))
            }
            fullWidth
          />
          <TextField
            label="AS-IS reports path"
            value={form.paths.asIsReportsPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, asIsReportsPath: event.target.value } }))
            }
            helperText="Persistent host/NAS path for pre-conversion analysis snapshots."
            fullWidth
          />
          <TextField
            label="Results reports path"
            value={form.paths.resultsReportsPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, resultsReportsPath: event.target.value } }))
            }
            helperText="Persistent host/NAS path for final conversion result reports."
            fullWidth
          />
          <TextField
            label="Logs path"
            value={form.paths.logsPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, logsPath: event.target.value } }))
            }
            helperText="Persistent host/NAS path for job logs that can be used later by IA analysis."
            fullWidth
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

export function AssetInventoryCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Asset Inventory</Typography>
            <Typography color="text.secondary" variant="body2">
              Discovery and reconciliation of Raw, Library, Converted, and Archive records.
            </Typography>
          </Stack>
          <FormControlLabel
            control={
              <Switch
                checked={form.assetInventory.autoSyncEnabled}
                onChange={(event) => setForm((current) => ({
                  ...current,
                  assetInventory: { ...current.assetInventory, autoSyncEnabled: event.target.checked },
                }))}
              />
            }
            label="Automatic inventory sync"
          />
          <TextField
            label="Sync interval minutes"
            type="number"
            value={form.assetInventory.syncIntervalMinutes}
            onChange={(event) => setForm((current) => ({
              ...current,
              assetInventory: { ...current.assetInventory, syncIntervalMinutes: Number(event.target.value) },
            }))}
            helperText="Runs after backend startup and then at this interval."
            inputProps={{ min: 5, max: 10080 }}
            disabled={!form.assetInventory.autoSyncEnabled}
            fullWidth
          />
          <TextField
            select
            label="Renamed or relocated assets"
            value={form.assetInventory.reconciliationMode}
            onChange={(event) => setForm((current) => ({
              ...current,
              assetInventory: {
                ...current.assetInventory,
                reconciliationMode: event.target.value as 'off' | 'review' | 'exact',
              },
            }))}
            helperText="Exact fingerprints can be reconciled automatically; uncertain legacy matches can require review."
            fullWidth
          >
            <MenuItem value="exact">Automatically reconcile exact fingerprints</MenuItem>
            <MenuItem value="review">Review all possible matches</MenuItem>
            <MenuItem value="off">Off</MenuItem>
          </TextField>
          <Alert severity="info">
            Inventory sync is read-only for media unless automatic Original Archive deletion is explicitly enabled in its own section.
          </Alert>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function StorageWorkspaceCard({ roles, workspace, saving, onSave, onDraftChange, draftOnly }: { roles: StorageRolesValue; workspace: WorkspaceValue; saving: boolean; onSave: (roles: Record<string, unknown>, workspace: WorkspaceValue) => void; onDraftChange?: (roles: StorageRolesValue, workspace: WorkspaceValue) => void; draftOnly?: boolean }) {
  const [draft, setDraft] = useSettingsDraftValue({ roles, workspace }, identitySettingsValue<{ roles: StorageRolesValue; workspace: WorkspaceValue }>, (next) => onDraftChange?.(next.roles, next.workspace));
  const roleDraft = draft.roles;
  const workspaceDraft = draft.workspace;
  return <Grid container spacing={2}>
    <Grid size={{ xs: 12, md: 7 }}><Card><CardContent><Stack spacing={2}><Stack><Typography variant="h3">Storage Roles</Typography><Typography color="text.secondary" variant="body2">Scheduler paths are referenced by role instead of host-specific locations.</Typography></Stack>
      {storageRoleNames.filter((role) => role !== 'originals_archive').map((role) => <TextField key={role} label={role.replaceAll('_', ' ')} value={roleDraft[role].path} onChange={(event) => setDraft((current) => ({ ...current, roles: { ...current.roles, [role]: { path: event.target.value } } }))} fullWidth />)}
      <Alert severity="info">The originals_archive role is managed in the Original Archive settings section.</Alert>
    </Stack></CardContent></Card></Grid>
    <Grid size={{ xs: 12, md: 5 }}><Card><CardContent><Stack spacing={2}><Stack><Typography variant="h3">Workspace Strategy</Typography><Typography color="text.secondary" variant="body2">Choose whether FFmpeg reads a work-disk copy or reads raw directly.</Typography></Stack>
      <TextField select label="Preferred mode" value={workspaceDraft.preferredMode} onChange={(event) => setDraft((current) => ({ ...current, workspace: { ...current.workspace, preferredMode: event.target.value as WorkspaceValue['preferredMode'] } }))}><MenuItem value="copy_to_work_disk">Copy input to work disk</MenuItem><MenuItem value="direct_mode">Read raw directly</MenuItem></TextField>
      <TextField select label="Insufficient workspace fallback" value={workspaceDraft.fallbackMode} onChange={(event) => setDraft((current) => ({ ...current, workspace: { ...current.workspace, fallbackMode: event.target.value as WorkspaceValue['fallbackMode'] } }))}><MenuItem value="wait">Wait for workspace</MenuItem><MenuItem value="direct_mode">Use direct mode</MenuItem></TextField>
      <FormControlLabel control={<Switch checked={workspaceDraft.allowDirectMode} onChange={(event) => setDraft((current) => ({ ...current, workspace: { ...current.workspace, allowDirectMode: event.target.checked } }))} />} label="Allow direct-mode fallback" />
      <FormControlLabel control={<Switch checked={workspaceDraft.estimateRequiredSpace} onChange={(event) => setDraft((current) => ({ ...current, workspace: { ...current.workspace, estimateRequiredSpace: event.target.checked } }))} />} label="Estimate required workspace" />
      <Alert severity="info">Copy mode reserves input + estimated output + temporary overhead. Insufficient space produces WAITING_SSD_SPACE.</Alert>
      {!draftOnly ? <Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(roleDraft as unknown as Record<string, unknown>, workspaceDraft)}>Save storage settings</Button> : null}
    </Stack></CardContent></Card></Grid>
  </Grid>;
}

export function workspaceValue(value: Record<string, unknown> | undefined): WorkspaceValue {
  return {
    preferredMode: value?.preferredMode === 'direct_mode' ? 'direct_mode' : 'copy_to_work_disk',
    fallbackMode: value?.fallbackMode === 'direct_mode' ? 'direct_mode' : 'wait',
    allowDirectMode: booleanValue(value?.allowDirectMode, false), estimateRequiredSpace: booleanValue(value?.estimateRequiredSpace, true),
  };
}

export function rebuildAssetTypes(current: SettingsForm, assetTypes: LibraryType[]): SettingsForm {
  return buildSettingsForm({ ...current, assetTypes });
}

export function summarizeLibrariesByType(libraries: Library[]) {
  return libraries.reduce<Record<string, number>>((summary, library) => {
    summary[library.type] = (summary[library.type] ?? 0) + 1;
    return summary;
  }, {});
}

export type AssetsStorageSettingsDomainProps = {
  form: SettingsForm;
  setForm: Dispatch<SetStateAction<SettingsForm>>;
  librariesByType: Record<string, number>;
  settings: AppSetting[];
  saving: boolean;
  hasLegacyAnalysisRecords: boolean;
  onAddType: () => void;
  onEditType: (type: LibraryType) => void;
  onDeleteType: (type: LibraryType) => void;
  onStorageDraftChange: (roles: StorageRolesValue, workspace: WorkspaceValue) => void;
};

export function AssetsStorageSettingsDomain({ form, setForm, librariesByType, settings, saving, hasLegacyAnalysisRecords, onAddType, onEditType, onDeleteType, onStorageDraftChange }: AssetsStorageSettingsDomainProps) {
  return <Stack spacing={2}>
    <AssetCategoriesPanel categories={form.assetCategories} onChange={(categories) => setForm((current) => buildSettingsForm({ ...current, assetCategories: normalizeStringArray(categories, current.assetCategories) }))} />
    <LibraryTypesPanel assetTypes={form.assetTypes} librariesByType={librariesByType} onAdd={onAddType} onEdit={onEditType} onDelete={onDeleteType} />
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 6 }}><PathsCard form={form} setForm={setForm} /></Grid>
      <Grid size={{ xs: 12, md: 6 }}><AssetInventoryCard form={form} setForm={setForm} /></Grid>
    </Grid>
    <StorageWorkspaceCard
      roles={storageRolesValue(form.ordinarySettings.storageRoles)}
      workspace={workspaceValue(form.ordinarySettings.workspace)}
      saving={saving}
      draftOnly
      onDraftChange={onStorageDraftChange}
      onSave={() => undefined}
    />
    {hasLegacyAnalysisRecords ? <Card variant="outlined" component="section" aria-label="Assets Settings" data-testid="assets-settings-section"><CardContent><Typography variant="h3" sx={{ mb: 1 }}>Assets Settings</Typography><LegacyAnalysisRecords settings={settings} /></CardContent></Card> : null}
  </Stack>;
}
