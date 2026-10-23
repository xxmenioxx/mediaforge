import { useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react';

export function useSettingsDraftValue<T>(
  value: unknown,
  normalize: (value: unknown) => T,
  onChange?: (value: T) => void,
): [T, Dispatch<SetStateAction<T>>] {
  const [draft, setDraft] = useState<T>(() => normalize(value));
  const baseline = useRef({ raw: value, normalized: normalize(value), serialized: JSON.stringify(value) });
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const firstDraftEffect = useRef(true);

  useEffect(() => {
    const serialized = JSON.stringify(value);
    if (serialized === baseline.current.serialized) return;
    baseline.current = { raw: value, normalized: normalize(value), serialized };
    setDraft(baseline.current.normalized);
  }, [normalize, value]);

  useEffect(() => {
    if (firstDraftEffect.current) {
      firstDraftEffect.current = false;
      return;
    }
    const serialized = JSON.stringify(draft);
    onChangeRef.current?.(serialized === JSON.stringify(baseline.current.normalized)
      ? baseline.current.raw as T
      : draft);
  }, [draft]);

  return [draft, setDraft];
}
