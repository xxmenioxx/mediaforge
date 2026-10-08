import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  FormControl,
  FormControlLabel,
  InputLabel,
  MenuItem,
  Pagination,
  Select,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { api } from '../api/client';
import type { OCRDictionaryEntry, OCRDictionaryLanguage, OCRDictionaryRuleInput } from '../api/types';

const languageNames: Record<OCRDictionaryLanguage, string> = {
  eng: 'English',
  spa: 'Spanish',
  jpn: 'Japanese',
  jpn_vert: 'Japanese (vertical)',
};

function emptyRule(language: OCRDictionaryLanguage): OCRDictionaryRuleInput {
  return {
    language,
    pattern: '',
    replacement: '',
    matchMode: 'literal',
    boundaryPolicy: 'word',
    leftContext: '',
    rightContext: '',
    casePolicy: 'exact',
    priority: 0,
    enabled: true,
  };
}

export function OCRDictionaryManager({ initialLanguage = 'eng' }: { initialLanguage?: OCRDictionaryLanguage }) {
  const [language, setLanguage] = useState(initialLanguage);
  const [search, setSearch] = useState('');
  const [sourceFilter, setSourceFilter] = useState<'all' | 'builtin' | 'user'>('all');
  const [enabledFilter, setEnabledFilter] = useState<'all' | 'enabled' | 'disabled'>('all');
  const [page, setPage] = useState(1);
  const [entries, setEntries] = useState<OCRDictionaryEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [editingID, setEditingID] = useState<string | null>(null);
  const [draft, setDraft] = useState<OCRDictionaryRuleInput>(() => emptyRule(initialLanguage));
  const [sample, setSample] = useState('');
  const [preview, setPreview] = useState<{ result: string; matchCount: number; ambiguousCount: number } | null>(null);
  const requestSequence = useRef(0);

  const refresh = useCallback(async () => {
    const sequence = ++requestSequence.current;
    setLoading(true);
    setError('');
    try {
      const response = await api.searchOCRDictionary({
        language,
        search,
        source: sourceFilter === 'all' ? undefined : sourceFilter,
        enabled: enabledFilter === 'all' ? undefined : enabledFilter === 'enabled',
        page,
        pageSize: 100,
      });
      if (sequence === requestSequence.current) {
        setEntries(response.entries);
        setTotal(response.total);
      }
    } catch (requestError) {
      if (sequence === requestSequence.current) {
        setError(requestError instanceof Error ? requestError.message : 'Could not load OCR dictionary.');
      }
    } finally {
      if (sequence === requestSequence.current) setLoading(false);
    }
  }, [enabledFilter, language, page, search, sourceFilter]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const lastPage = Math.max(1, Math.ceil(total / 100));
    if (page > lastPage) setPage(lastPage);
  }, [page, total]);

  function beginCreate() {
    setEditingID(null);
    setDraft(emptyRule(language));
    setSample('');
    setPreview(null);
    setError('');
    setNotice('');
  }

  function beginEdit(entry: OCRDictionaryEntry) {
    setEditingID(entry.id);
    setDraft({
      language: entry.language,
      pattern: entry.pattern,
      replacement: entry.replacement,
      matchMode: 'literal',
      boundaryPolicy: 'word',
      leftContext: entry.leftContext ?? '',
      rightContext: entry.rightContext ?? '',
      casePolicy: entry.casePolicy,
      priority: entry.priority,
      enabled: entry.enabled,
    });
    setSample('');
    setPreview(null);
    setError('');
    setNotice('');
  }

  function updateDraft<K extends keyof OCRDictionaryRuleInput>(key: K, value: OCRDictionaryRuleInput[K]) {
    setDraft((current) => ({ ...current, [key]: value }));
    setPreview(null);
    setError('');
  }

  async function saveRule() {
    setSaving(true);
    setError('');
    setNotice('');
    try {
      if (editingID) {
        await api.updateOCRDictionaryEntry(editingID, draft);
        setNotice('Rule updated.');
      } else {
        await api.createOCRDictionaryEntry(draft);
        setNotice('Rule added.');
      }
      setEditingID(null);
      setDraft(emptyRule(language));
      await refresh();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Could not save OCR dictionary rule.');
    } finally {
      setSaving(false);
    }
  }

  async function toggleEntry(entry: OCRDictionaryEntry, enabled: boolean) {
    setError('');
    setNotice('');
    try {
      await api.setOCRDictionaryEntryEnabled(entry.id, enabled);
      setNotice(enabled ? 'Rule enabled.' : 'Rule disabled.');
      await refresh();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Could not change rule state.');
    }
  }

  async function deleteEntry(entry: OCRDictionaryEntry) {
    setError('');
    setNotice('');
    try {
      await api.deleteOCRDictionaryEntry(entry.id);
      if (editingID === entry.id) beginCreate();
      setNotice('Rule deleted.');
      await refresh();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Could not delete OCR dictionary rule.');
    }
  }

  async function testRule() {
    setSaving(true);
    setError('');
    setNotice('');
    try {
      const result = await api.testOCRDictionaryMatch({ language: draft.language, sample, rule: draft });
      setPreview({ result: result.result, matchCount: result.matchCount, ambiguousCount: result.ambiguous?.length ?? 0 });
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Could not preview this rule.');
    } finally {
      setSaving(false);
    }
  }

  function changeLanguage(next: OCRDictionaryLanguage) {
    setLanguage(next);
    setPage(1);
    setEditingID(null);
    setDraft(emptyRule(next));
    setPreview(null);
  }

  return (
    <Stack spacing={2}>
      <Box>
        <Typography variant="h3">Contextual OCR dictionary</Typography>
        <Typography variant="body2" color="text.secondary">
          Manage literal, language-scoped suggestions. Rules use word boundaries and are validated by the server.
        </Typography>
      </Box>

      <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5}>
        <FormControl size="small" sx={{ minWidth: 190 }}>
          <InputLabel id="ocr-dictionary-language-label">Language</InputLabel>
          <Select labelId="ocr-dictionary-language-label" label="Language" value={language} onChange={(event) => changeLanguage(event.target.value as OCRDictionaryLanguage)}>
            {Object.entries(languageNames).map(([value, label]) => <MenuItem key={value} value={value}>{label}</MenuItem>)}
          </Select>
        </FormControl>
        <TextField size="small" label="Search rules" value={search} onChange={(event) => { setSearch(event.target.value); setPage(1); }} fullWidth />
        <FormControl size="small" sx={{ minWidth: 150 }}>
          <InputLabel id="ocr-dictionary-source-label">Source</InputLabel>
          <Select labelId="ocr-dictionary-source-label" label="Source" value={sourceFilter} onChange={(event) => { setSourceFilter(event.target.value as typeof sourceFilter); setPage(1); }}>
            <MenuItem value="all">All sources</MenuItem><MenuItem value="builtin">Built-in</MenuItem><MenuItem value="user">Custom</MenuItem>
          </Select>
        </FormControl>
        <FormControl size="small" sx={{ minWidth: 150 }}>
          <InputLabel id="ocr-dictionary-enabled-label">State</InputLabel>
          <Select labelId="ocr-dictionary-enabled-label" label="State" value={enabledFilter} onChange={(event) => { setEnabledFilter(event.target.value as typeof enabledFilter); setPage(1); }}>
            <MenuItem value="all">All states</MenuItem><MenuItem value="enabled">Enabled</MenuItem><MenuItem value="disabled">Disabled</MenuItem>
          </Select>
        </FormControl>
      </Stack>

      {error ? <Alert severity="error">{error}</Alert> : null}
      {notice ? <Alert severity="success">{notice}</Alert> : null}

      <Stack direction="row" alignItems="center" justifyContent="space-between">
        <Typography variant="body2" color="text.secondary">{total} rules · {languageNames[language]}</Typography>
        <Button variant="contained" onClick={beginCreate}>Add custom rule</Button>
      </Stack>

      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small" aria-label="OCR dictionary rules">
          <TableHead><TableRow><TableCell>Rule</TableCell><TableCell>Context</TableCell><TableCell>Priority</TableCell><TableCell>Source</TableCell><TableCell>Enabled</TableCell><TableCell>Actions</TableCell></TableRow></TableHead>
          <TableBody>
            {entries.map((entry) => (
              <TableRow key={entry.id}>
                <TableCell><Typography fontWeight={600}>{entry.pattern} → {entry.replacement}</Typography><Typography variant="caption" color="text.secondary">{entry.casePolicy} case · v{entry.ruleVersion}</Typography></TableCell>
                <TableCell>{entry.leftContext || entry.rightContext ? `${entry.leftContext || '…'} [rule] ${entry.rightContext || '…'}` : 'Word boundary'}</TableCell>
                <TableCell>{entry.priority}</TableCell>
                <TableCell>{entry.source === 'builtin' ? 'Built-in' : 'Custom'}</TableCell>
                <TableCell><FormControlLabel label={`Enable ${entry.pattern}`} control={<Switch size="small" checked={entry.enabled} onChange={(event) => void toggleEntry(entry, event.target.checked)} inputProps={{ role: 'switch', 'aria-label': `Enable ${entry.pattern}` }} />} /></TableCell>
                <TableCell>
                  <Stack direction="row" spacing={0.5}>
                    {entry.source === 'user' ? <><Button size="small" onClick={() => beginEdit(entry)}>Edit</Button><Button size="small" color="error" onClick={() => void deleteEntry(entry)}>Delete</Button></> : <Typography variant="caption" color="text.secondary">Protected</Typography>}
                  </Stack>
                </TableCell>
              </TableRow>
            ))}
            {!loading && entries.length === 0 ? <TableRow><TableCell colSpan={6}>No matching rules.</TableCell></TableRow> : null}
          </TableBody>
        </Table>
      </Box>
      <Stack direction="row" justifyContent="flex-end">
        <Pagination count={Math.max(1, Math.ceil(total / 100))} page={page} onChange={(_, nextPage) => setPage(nextPage)} disabled={loading} size="small" showFirstButton showLastButton />
      </Stack>
      {loading ? <Stack direction="row" spacing={1} alignItems="center"><CircularProgress size={18} /><Typography variant="body2">Loading dictionary…</Typography></Stack> : null}

      <Box component="section" sx={{ p: 2, border: 1, borderColor: 'divider', borderRadius: 1 }}>
        <Stack spacing={1.5}>
          <Typography variant="h4">{editingID ? 'Edit custom rule' : 'Custom rule'}</Typography>
          <Typography variant="body2" color="text.secondary">{languageNames[draft.language]} · literal match · word boundary</Typography>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5}>
            <TextField label="OCR text" value={draft.pattern} onChange={(event) => updateDraft('pattern', event.target.value)} required fullWidth />
            <TextField label="Suggested text" value={draft.replacement} onChange={(event) => updateDraft('replacement', event.target.value)} required fullWidth />
          </Stack>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5}>
            <TextField label="Required text before" value={draft.leftContext ?? ''} onChange={(event) => updateDraft('leftContext', event.target.value)} fullWidth />
            <TextField label="Required text after" value={draft.rightContext ?? ''} onChange={(event) => updateDraft('rightContext', event.target.value)} fullWidth />
            <FormControl size="small" sx={{ minWidth: 170 }}>
              <InputLabel id="ocr-dictionary-case-label">Case</InputLabel>
              <Select labelId="ocr-dictionary-case-label" label="Case" value={draft.casePolicy} onChange={(event) => updateDraft('casePolicy', event.target.value as OCRDictionaryRuleInput['casePolicy'])}>
                <MenuItem value="exact">Exact</MenuItem><MenuItem value="insensitive">Ignore case</MenuItem>
              </Select>
            </FormControl>
            <TextField label="Priority" type="number" value={draft.priority} onChange={(event) => updateDraft('priority', Number(event.target.value))} inputProps={{ min: -1000, max: 1000 }} sx={{ width: 130 }} />
          </Stack>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} alignItems={{ md: 'flex-start' }}>
            <TextField label="Sample subtitle text" value={sample} onChange={(event) => { setSample(event.target.value); setPreview(null); }} fullWidth multiline minRows={2} />
            <Stack direction="row" spacing={1}>
              <Button variant="outlined" onClick={() => void testRule()} disabled={saving || !sample.trim() || !draft.pattern.trim() || !draft.replacement.trim()}>Test match</Button>
              <Button variant="contained" onClick={() => void saveRule()} disabled={saving || !draft.pattern.trim() || !draft.replacement.trim()}>{editingID ? 'Save rule' : 'Add rule'}</Button>
              {editingID ? <Button onClick={beginCreate}>Cancel</Button> : null}
            </Stack>
          </Stack>
          {saving ? <CircularProgress size={18} /> : null}
          {preview ? <Alert severity={preview.ambiguousCount > 0 ? 'warning' : 'info'}>
            <Typography variant="body2">{preview.matchCount} match(es){preview.ambiguousCount ? ` · ${preview.ambiguousCount} ambiguous overlap(s)` : ''}</Typography>
            <Typography component="pre" variant="body2" sx={{ whiteSpace: 'pre-wrap', fontFamily: 'inherit', mb: 0 }}>{preview.result}</Typography>
          </Alert> : null}
        </Stack>
      </Box>
    </Stack>
  );
}
