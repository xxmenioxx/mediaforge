import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { api } from '../api/client';
import type { OCRCorrectionMemoryEntry, OCRCorrectionMemoryScopeType } from '../api/types';

export type OCRCorrectionMemorySettingsValue = {
  enabled: boolean;
  retentionDays: number;
  includeInAIPrompts: boolean;
};

export function ocrCorrectionMemorySettingsValue(value?: Record<string, unknown>): OCRCorrectionMemorySettingsValue {
  return {
    enabled: value?.enabled === true,
    retentionDays: typeof value?.retentionDays === 'number' ? value.retentionDays : 180,
    includeInAIPrompts: value?.includeInAIPrompts === true,
  };
}

const scopeTypes: OCRCorrectionMemoryScopeType[] = ['global', 'library', 'series', 'title', 'asset'];
const memoryQueryKey = ['ocrCorrectionMemory'] as const;
const pageSize = 20;

type EditDraft = Pick<OCRCorrectionMemoryEntry, 'originalText' | 'correctedText' | 'scopeType' | 'scopeId'>;

export function OCRCorrectionMemorySettings({
  value,
  onDraftChange,
}: {
  value: OCRCorrectionMemorySettingsValue;
  onDraftChange: (value: Record<string, unknown>) => void;
}) {
  const retentionValid = Number.isInteger(value.retentionDays) && value.retentionDays >= 30 && value.retentionDays <= 3650;
  return (
    <>
      <Card>
        <CardContent>
          <Stack spacing={1.5}>
            <Stack>
              <Typography variant="h3">OCR correction memory</Typography>
              <Typography color="text.secondary" variant="body2">
                Keep a local, user-owned memory of approved manual and dictionary-applied cue corrections. Entries are suggestions and do not train model weights.
              </Typography>
            </Stack>
            <FormControlLabel
              control={(
                <Switch
                  checked={value.enabled}
                  onChange={(event) => onDraftChange({ ...value, enabled: event.target.checked })}
                />
              )}
              label="Learn from approved OCR corrections"
            />
            <TextField
              type="number"
              label="Correction memory retention (days)"
              value={value.retentionDays}
              inputProps={{ min: 30, max: 3650, step: 1 }}
              error={!retentionValid}
              helperText={retentionValid ? 'Choose from 30 to 3,650 days.' : 'Enter a whole number from 30 to 3,650.'}
              onChange={(event) => onDraftChange({ ...value, retentionDays: event.target.value === '' ? Number.NaN : Number(event.target.value) })}
            />
            <FormControlLabel
              control={(
                <Switch
                  checked={value.includeInAIPrompts}
                  disabled={!value.enabled}
                  onChange={(event) => onDraftChange({ ...value, includeInAIPrompts: event.target.checked })}
                />
              )}
              label="Include bounded matches in optional OCR AI prompts"
            />
            <Typography color="text.secondary" variant="caption">
              Prompt enrichment is a separate choice and does not enable OCR AI. Turning learning off preserves existing entries.
            </Typography>
          </Stack>
        </CardContent>
      </Card>
      <OCRCorrectionMemoryManager />
    </>
  );
}

function downloadMemoryExport(exportedAt: string, entries: OCRCorrectionMemoryEntry[]) {
  const blob = new Blob([JSON.stringify({ exportedAt, entries }, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `ocr-correction-memory-${new Date(exportedAt).toISOString().slice(0, 10)}.json`;
  link.click();
  URL.revokeObjectURL(url);
}

export function OCRCorrectionMemoryManager() {
  const queryClient = useQueryClient();
  const [expanded, setExpanded] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [language, setLanguage] = useState('');
  const [scopeType, setScopeType] = useState<OCRCorrectionMemoryScopeType | ''>('');
  const [page, setPage] = useState(1);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editDraft, setEditDraft] = useState<EditDraft | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<OCRCorrectionMemoryEntry | null>(null);
  const entriesQuery = useQuery({
    queryKey: [...memoryQueryKey, search, language, scopeType, page],
    queryFn: () => api.searchOCRCorrectionMemory({ search, language: language || undefined, scopeType: scopeType || undefined, page, pageSize }),
    enabled: expanded,
  });
  const invalidateEntries = () => queryClient.invalidateQueries({ queryKey: memoryQueryKey });
  const editEntry = useMutation({
    mutationFn: ({ id, draft }: { id: number; draft: EditDraft }) => api.updateOCRCorrectionMemoryEntry(id, draft),
    onSuccess: () => {
      setEditingId(null);
      setEditDraft(null);
      void invalidateEntries();
    },
  });
  const setEntryEnabled = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) => api.setOCRCorrectionMemoryEntryEnabled(id, enabled),
    onSuccess: () => void invalidateEntries(),
  });
  const deleteEntry = useMutation({
    mutationFn: (id: number) => api.deleteOCRCorrectionMemoryEntry(id),
    onSuccess: () => {
      setDeleteTarget(null);
      void invalidateEntries();
    },
  });
  const exportEntries = useMutation({
    mutationFn: api.exportOCRCorrectionMemory,
    onSuccess: (result) => downloadMemoryExport(result.exportedAt, result.entries),
  });
  const startEdit = (entry: OCRCorrectionMemoryEntry) => {
    setEditingId(entry.id);
    setEditDraft({
      originalText: entry.originalText,
      correctedText: entry.correctedText,
      scopeType: entry.scopeType,
      scopeId: entry.scopeId,
    });
  };

  return (
    <Accordion expanded={expanded} onChange={(_event, nextExpanded) => setExpanded(nextExpanded)}>
      <AccordionSummary expandIcon={<ExpandMoreIcon />} aria-controls="ocr-correction-memory-content" id="ocr-correction-memory-header">
        <Stack>
          <Typography variant="h3">OCR correction memory manager</Typography>
          <Typography color="text.secondary" variant="body2">Inspect, search, edit, revoke, export, or delete approved examples.</Typography>
        </Stack>
      </AccordionSummary>
      <AccordionDetails>
        <Stack spacing={1.5}>
          <Alert severity="info">Stored examples retain cue text and bounded provenance only. The memory does not retain PGS images or full transcripts.</Alert>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
            <TextField
              size="small"
              label="Search correction memory"
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  setSearch(searchInput.trim());
                  setPage(1);
                }
              }}
              fullWidth
            />
            <TextField
              size="small"
              label="Language"
              value={language}
              onChange={(event) => { setLanguage(event.target.value.trim()); setPage(1); }}
              sx={{ minWidth: { sm: 130 } }}
            />
            <TextField
              select
              size="small"
              label="Scope"
              value={scopeType}
              onChange={(event) => { setScopeType(event.target.value as OCRCorrectionMemoryScopeType | ''); setPage(1); }}
              sx={{ minWidth: { sm: 150 } }}
            >
              <MenuItem value="">All scopes</MenuItem>
              {scopeTypes.map((scope) => <MenuItem key={scope} value={scope}>{scope}</MenuItem>)}
            </TextField>
            <Button variant="outlined" onClick={() => { setSearch(searchInput.trim()); setPage(1); }}>Search</Button>
            <Button variant="outlined" onClick={() => exportEntries.mutate()} disabled={exportEntries.isPending}>
              {exportEntries.isPending ? 'Exporting…' : 'Export memory'}
            </Button>
          </Stack>
          {entriesQuery.isError ? <Alert severity="warning">{entriesQuery.error instanceof Error ? entriesQuery.error.message : 'Correction memory could not be loaded.'}</Alert> : null}
          {editEntry.isError || setEntryEnabled.isError || deleteEntry.isError || exportEntries.isError ? (
            <Alert severity="warning">{[editEntry.error, setEntryEnabled.error, deleteEntry.error, exportEntries.error].find((error) => error instanceof Error)?.message ?? 'Correction memory action failed.'}</Alert>
          ) : null}
          {entriesQuery.isLoading ? <Typography color="text.secondary">Loading correction memory…</Typography> : null}
          {(entriesQuery.data?.entries ?? []).length === 0 && !entriesQuery.isLoading && !entriesQuery.isError ? (
            <Typography color="text.secondary">No approved correction entries match this search.</Typography>
          ) : null}
          {(entriesQuery.data?.entries ?? []).map((entry) => {
            const isEditing = editingId === entry.id && editDraft !== null;
            return (
              <Card variant="outlined" key={entry.id}>
                <CardContent>
                  <Stack spacing={1}>
                    <Stack direction="row" spacing={0.75} alignItems="center" flexWrap="wrap" useFlexGap>
                      <Typography fontWeight={700}>Entry {entry.id} · {entry.sourceLanguage.toUpperCase()}</Typography>
                      <Chip size="small" label={`${entry.scopeType}: ${entry.scopeId || 'all'}`} />
                      <Chip size="small" label={entry.approvalValid ? 'Approval valid' : 'Approval stale'} color={entry.approvalValid ? 'success' : 'warning'} />
                      <Chip size="small" label={`${entry.acceptedSuggestions} accepted · ${entry.rejectedSuggestions} rejected`} />
                      <Chip size="small" label={`${entry.occurrenceCount} occurrence${entry.occurrenceCount === 1 ? '' : 's'}`} />
                    </Stack>
                    {isEditing ? (
                      <>
                        <TextField label="Original text" multiline minRows={2} value={editDraft.originalText} onChange={(event) => setEditDraft({ ...editDraft, originalText: event.target.value })} />
                        <TextField label="Corrected text" multiline minRows={2} value={editDraft.correctedText} onChange={(event) => setEditDraft({ ...editDraft, correctedText: event.target.value })} />
                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                          <TextField select label="Scope type" value={editDraft.scopeType} onChange={(event) => setEditDraft({ ...editDraft, scopeType: event.target.value as OCRCorrectionMemoryScopeType })}>
                            {scopeTypes.map((scope) => <MenuItem key={scope} value={scope}>{scope}</MenuItem>)}
                          </TextField>
                          <TextField label="Scope ID" value={editDraft.scopeId} onChange={(event) => setEditDraft({ ...editDraft, scopeId: event.target.value })} fullWidth />
                        </Stack>
                        <Stack direction="row" spacing={1}>
                          <Button onClick={() => editEntry.mutate({ id: entry.id, draft: editDraft })} disabled={editEntry.isPending || !editDraft.originalText.trim() || !editDraft.correctedText.trim()}>
                            {editEntry.isPending ? 'Saving…' : 'Save entry'}
                          </Button>
                          <Button onClick={() => { setEditingId(null); setEditDraft(null); }} disabled={editEntry.isPending}>Cancel</Button>
                        </Stack>
                      </>
                    ) : (
                      <>
                        <Typography variant="body2"><strong>Original:</strong> {entry.originalText}</Typography>
                        <Typography variant="body2"><strong>Corrected:</strong> {entry.correctedText}</Typography>
                        {entry.supersededById ? <Alert severity="info">This example was overridden by entry {entry.supersededById}. Re-enable it to restore this approved correction for its scope.</Alert> : null}
                        {!entry.approvalValid ? <Alert severity="warning">Text or source changed after approval. Review the entry to create a new approved example; stale approval cannot be re-enabled.</Alert> : null}
                        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
                          <FormControlLabel
                            control={(
                              <Switch
                                checked={entry.enabled}
                                disabled={!entry.approvalValid || setEntryEnabled.isPending}
                                onChange={(event) => setEntryEnabled.mutate({ id: entry.id, enabled: event.target.checked })}
                                inputProps={{ 'aria-label': `Enable OCR correction memory entry ${entry.id}` }}
                              />
                            )}
                            label="Use for suggestions"
                          />
                          <Button onClick={() => startEdit(entry)}>Edit entry</Button>
                          <Button color="error" onClick={() => setDeleteTarget(entry)}>Delete entry</Button>
                        </Stack>
                      </>
                    )}
                    <Typography variant="caption" color="text.secondary">Approved {new Date(entry.approvedAt).toLocaleString()} · expires {new Date(entry.expiresAt).toLocaleString()} · context {entry.contextFingerprint}</Typography>
                    <details>
                      <summary>Provenance ({entry.provenance.length})</summary>
                      <Stack spacing={0.5} sx={{ mt: 1 }}>
                        {entry.provenance.map((source, index) => (
                          <Typography variant="caption" key={`${source.sourceRevision}-${source.artifactRevision}-${index}`}>
                            {source.approvalAction} · cue {source.cueId} · source {source.sourceRevision} · artifact {source.artifactRevision} · hash {source.sourceHash}{source.dictionaryRuleId ? ` · dictionary rule ${source.dictionaryRuleId} v${source.dictionaryRuleVersion ?? 'unknown'}` : ''}{source.modelIdentifier ? ` · model ${source.modelIdentifier}` : ''}
                          </Typography>
                        ))}
                      </Stack>
                    </details>
                  </Stack>
                </CardContent>
              </Card>
            );
          })}
          <Stack direction="row" spacing={1} alignItems="center">
            <Button onClick={() => setPage((current) => Math.max(1, current - 1))} disabled={page <= 1 || entriesQuery.isFetching}>Previous</Button>
            <Typography variant="body2">Page {entriesQuery.data?.page ?? page} of {Math.max(1, Math.ceil((entriesQuery.data?.total ?? 0) / pageSize))} · {entriesQuery.data?.total ?? 0} entries</Typography>
            <Button onClick={() => setPage((current) => current + 1)} disabled={entriesQuery.isFetching || page * pageSize >= (entriesQuery.data?.total ?? 0)}>Next</Button>
          </Stack>
        </Stack>
      </AccordionDetails>
      <Dialog open={deleteTarget !== null} onClose={() => deleteEntry.isPending ? undefined : setDeleteTarget(null)}>
        <DialogTitle>Delete OCR correction memory entry?</DialogTitle>
        <DialogContent>
          <Typography>Delete entry {deleteTarget?.id} (“{deleteTarget?.originalText}” → “{deleteTarget?.correctedText}”)? This removes it from future suggestions and cannot be undone.</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteTarget(null)} disabled={deleteEntry.isPending}>Cancel</Button>
          <Button color="error" onClick={() => { if (deleteTarget) deleteEntry.mutate(deleteTarget.id); }} disabled={!deleteTarget || deleteEntry.isPending}>
            {deleteEntry.isPending ? 'Deleting…' : 'Delete entry'}
          </Button>
        </DialogActions>
      </Dialog>
    </Accordion>
  );
}
