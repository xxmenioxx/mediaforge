export type QsvQualityRange = {
  min: number;
  max: number;
  recommended: number;
};

export function qsvQualityRangeForCrf(crf: number): QsvQualityRange {
  const normalized = Math.min(30, Math.max(14, Math.round(crf)));
  const recommended = normalized <= 20
    ? Math.max(15, normalized - 2)
    : Math.min(30, 18 + Math.round((normalized - 20) * 1.5));
  return {
    min: Math.max(1, recommended - 1),
    max: Math.min(51, recommended + 1),
    recommended,
  };
}

export function qsvQualityHelper(crf: number) {
  const range = qsvQualityRangeForCrf(crf);
  return `QSV starting range for software CRF ${crf}: ICQ ${range.min}–${range.max} (start at ${range.recommended}). Validate a representative preview; this is not an exact equivalence.`;
}
