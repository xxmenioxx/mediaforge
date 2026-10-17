import { useState } from 'react';
import { Alert, Box, Button, Chip, Collapse, Stack, Tab, Tabs, Typography } from '@mui/material';
import type { ValidationFinding, ValidationOverride } from '../../api/types';
import { ValidationFindingDetails, ValidationFindingStatus } from './ValidationRulePresentation';
import { orderValidationFindings, validationFindingDiagnosis, validationRuleLabel } from './validationRuleFormatters';

type FindingFilter = 'attention' | 'passed' | 'all';

function unresolved(finding: ValidationFinding): boolean {
  return finding.state !== 'PASS' && finding.state !== 'NOT_APPLICABLE';
}

export function ValidationFindingsTable({
  findings,
  overridesByKey,
  onSkip,
}: {
  findings: ValidationFinding[];
  overridesByKey: Map<string, ValidationOverride>;
  onSkip: (finding: ValidationFinding) => void;
}) {
  const [filter, setFilter] = useState<FindingFilter>('attention');
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const ordered = orderValidationFindings(findings);
  const attentionCount = findings.filter(unresolved).length;
  const passedCount = findings.filter((finding) => finding.state === 'PASS').length;
  const visible = ordered.filter((finding) => {
    if (filter === 'passed') return finding.state === 'PASS';
    if (filter === 'attention') return unresolved(finding);
    return true;
  });

  return <Stack spacing={1.5}>
    <Stack spacing={0.5}>
      <Typography variant="h6">Diagnostics</Typography>
      <Typography variant="body2" color="text.secondary">Canonical finding results remain visible even when a separate manual override is recorded.</Typography>
    </Stack>
    {findings.length === 0
      ? <Alert severity="info">This run contains no validation findings.</Alert>
      : <>
        <Tabs value={filter} onChange={(_, value: FindingFilter) => { setFilter(value); setExpandedKey(null); }} aria-label="Filter validation findings" variant="scrollable" scrollButtons="auto">
          <Tab value="attention" label={`Needs attention (${attentionCount})`} />
          <Tab value="passed" label={`Passed (${passedCount})`} />
          <Tab value="all" label={`All checks (${findings.length})`} />
        </Tabs>
        {visible.length === 0
          ? <Alert severity="success">No findings need attention. Passed checks remain available in the Passed filter.</Alert>
          : <Box role="table" aria-label="Validation diagnostics">
            <Box role="rowgroup" sx={{ display: { xs: 'none', md: 'block' } }}>
              <Box role="row" sx={{ display: 'grid', gridTemplateColumns: 'minmax(160px, 1fr) 145px minmax(260px, 2fr) 155px', gap: 1, px: 1.5, py: 1, borderBottom: 1, borderColor: 'divider' }}>
                {['Check', 'Status', 'Diagnosis', 'Action'].map((heading) => <Typography key={heading} role="columnheader" variant="overline" color="text.secondary">{heading}</Typography>)}
              </Box>
            </Box>
            <Box role="rowgroup">
              {visible.map((finding, index) => {
                const override = overridesByKey.get(finding.key);
                const expanded = expandedKey === finding.key;
                const label = validationRuleLabel(finding.key);
                const detailId = `validation-finding-detail-${index}`;
                return <Box key={finding.key}>
                  <Box role="row" sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr) auto', md: 'minmax(160px, 1fr) 145px minmax(260px, 2fr) 155px' }, gap: { xs: 1, md: 1 }, px: { xs: 1, sm: 1.5 }, py: 1.25, borderBottom: 1, borderColor: 'divider', alignItems: 'start' }}>
                    <Box role="cell" sx={{ minWidth: 0 }}>
                      <Typography variant="caption" display={{ xs: 'block', md: 'none' }} color="text.secondary">Check</Typography>
                      <Typography fontWeight={700} sx={{ overflowWrap: 'anywhere' }}>{label}</Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>{finding.key}</Typography>
                    </Box>
                    <Box role="cell" sx={{ minWidth: 0 }}>
                      <Typography variant="caption" display={{ xs: 'block', md: 'none' }} color="text.secondary">Status</Typography>
                      <Stack spacing={0.5} alignItems="flex-start">
                        <ValidationFindingStatus state={finding.state} />
                        {override ? <Chip size="small" color="info" label="SKIPPED" title="Manual override recorded; canonical finding is unchanged." /> : null}
                      </Stack>
                    </Box>
                    <Box role="cell" sx={{ gridColumn: { xs: '1 / -1', md: 'auto' }, minWidth: 0 }}>
                      <Typography variant="caption" display={{ xs: 'block', md: 'none' }} color="text.secondary">Diagnosis</Typography>
                      <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>{validationFindingDiagnosis(finding)}</Typography>
                    </Box>
                    <Box role="cell" sx={{ gridColumn: { xs: '1 / -1', md: 'auto' }, minWidth: 0 }}>
                      <Typography variant="caption" display={{ xs: 'block', md: 'none' }} color="text.secondary">Action</Typography>
                      <Stack direction="row" spacing={0.5} useFlexGap flexWrap="wrap">
                        <Button size="small" aria-label={`${expanded ? 'Hide' : 'Show'} details for ${label}`} aria-expanded={expanded} aria-controls={detailId} onClick={() => setExpandedKey(expanded ? null : finding.key)}>
                          {expanded ? 'Hide details' : 'Details'}
                        </Button>
                        {finding.overrideable && unresolved(finding) && !override
                          ? <Button size="small" variant="outlined" onClick={() => onSkip(finding)}>Skip</Button>
                          : null}
                      </Stack>
                    </Box>
                  </Box>
                  <Collapse in={expanded} unmountOnExit>
                    <Box id={detailId} role="row" sx={{ bgcolor: 'action.hover' }}>
                      <Box role="cell" sx={{ gridColumn: '1 / -1' }}>
                        <ValidationFindingDetails finding={finding} override={override} />
                      </Box>
                    </Box>
                  </Collapse>
                </Box>;
              })}
            </Box>
          </Box>}
      </>}
  </Stack>;
}
