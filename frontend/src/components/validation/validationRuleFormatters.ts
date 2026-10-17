import type { ValidationFinding } from '../../api/types';

const knownRuleLabels: Record<string, string> = {
  'artifact.integrity': 'Output integrity',
  'audio.tracks_actions': 'Audio track actions',
  'duration.timeline': 'Timeline duration',
  'timeline.av_alignment': 'Audio/video timing alignment',
  'video.properties': 'Video properties',
  'subtitles.streams_actions': 'Subtitle stream actions',
};

const stateOrder: Record<ValidationFinding['state'], number> = {
  FAIL: 0,
  WARNING: 1,
  UNVERIFIED: 2,
  REVIEW_REQUIRED: 3,
  PASS: 4,
  NOT_APPLICABLE: 5,
};

const severityOrder: Record<ValidationFinding['severity'], number> = {
  critical: 0,
  error: 1,
  warning: 2,
  info: 3,
};

export function validationRuleLabel(key: string): string {
  const fallback = key.split(/[._]/).filter(Boolean).map((part) => part[0]?.toUpperCase() + part.slice(1)).join(' ') || key;
  return knownRuleLabels[key] ?? fallback;
}

export function validationFindingDiagnosis(finding: ValidationFinding): string {
  const explanation = finding.explanation.trim();
  if (explanation) return explanation;
  if (finding.state === 'UNVERIFIED') return 'Evidence is incomplete; this check could not be verified.';
  if (finding.state === 'REVIEW_REQUIRED') return 'This check needs a review of the available evidence.';
  if (finding.state === 'NOT_APPLICABLE') return 'This check does not apply to the current output.';
  if (finding.state === 'PASS') return 'This check passed using the evidence recorded for this run.';
  if (finding.state === 'WARNING') return 'Validation recorded a warning for this check.';
  return 'This check failed.';
}

export function orderValidationFindings(findings: ValidationFinding[]): ValidationFinding[] {
  return findings.map((finding, index) => ({ finding, index })).sort((left, right) =>
    stateOrder[left.finding.state] - stateOrder[right.finding.state]
      || severityOrder[left.finding.severity] - severityOrder[right.finding.severity]
      || left.index - right.index,
  ).map(({ finding }) => finding);
}

export type EvidenceField = { label: string; value: string };

function humanizeEvidenceKey(key: string): string {
  const unitMatch = key.match(/(?:_|)(Ms|Milliseconds|Seconds|Frames)$/i);
  const base = unitMatch ? key.slice(0, -unitMatch[0].length) : key;
  const label = base
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[._-]+/g, ' ')
    .trim()
    .replace(/\b\w/g, (letter) => letter.toUpperCase());
  if (!unitMatch) return label || key;
  const unit = unitMatch[1].toLowerCase() === 'milliseconds' || unitMatch[1].toLowerCase() === 'ms'
    ? 'ms'
    : unitMatch[1].toLowerCase() === 'seconds' ? 's' : 'frames';
  return `${label} (${unit})`;
}

function scalarText(value: unknown): string | undefined {
  if (typeof value === 'string') return value.length > 0 ? value : 'Empty value';
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return undefined;
}

export function formatEvidenceFields(value: unknown, rootLabel: string): EvidenceField[] {
  const fields: EvidenceField[] = [];
  const maxFields = 100;

  const visit = (current: unknown, path: string[], depth: number) => {
    if (fields.length >= maxFields) return;
    const scalar = scalarText(current);
    if (scalar !== undefined) {
      fields.push({ label: path.map(humanizeEvidenceKey).join(' · ') || rootLabel, value: scalar });
      return;
    }
    if (current === null || current === undefined) {
      fields.push({ label: path.map(humanizeEvidenceKey).join(' · ') || rootLabel, value: 'Not recorded' });
      return;
    }
    if (depth >= 6) {
      fields.push({ label: path.map(humanizeEvidenceKey).join(' · ') || rootLabel, value: 'Nested evidence is available in Technical Evidence.' });
      return;
    }
    if (Array.isArray(current)) {
      if (current.length === 0) {
        fields.push({ label: path.map(humanizeEvidenceKey).join(' · ') || rootLabel, value: 'No items recorded' });
      }
      current.forEach((item, index) => {
        const parent = path[path.length - 1]?.toLowerCase() ?? '';
        const itemLabel = parent.includes('track') || parent === 'audio' || parent === 'video'
          ? `Track ${index + 1}`
          : `Item ${index + 1}`;
        visit(item, [...path, itemLabel], depth + 1);
      });
      return;
    }
    if (typeof current === 'object') {
      const entries = Object.entries(current as Record<string, unknown>);
      if (entries.length === 0) {
        fields.push({ label: path.map(humanizeEvidenceKey).join(' · ') || rootLabel, value: 'No fields recorded' });
        return;
      }
      entries.forEach(([key, nested]) => visit(nested, [...path, key], depth + 1));
    }
  };

  visit(value, [], 0);
  if (fields.length >= maxFields) fields.push({ label: 'Additional evidence', value: 'More fields are available in Technical Evidence.' });
  return fields;
}
