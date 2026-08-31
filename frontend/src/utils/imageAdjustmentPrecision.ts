export function formatFilterNumber(value: number) {
  const rounded = Number(value.toFixed(4));
  return String(Object.is(rounded, -0) ? 0 : rounded);
}

export function exposureFromStored(value: number) {
  return value / 100;
}

export function exposureToStored(value: number) {
  return value * 100;
}

export function brightnessFromStored(value: number) {
  return (value / 100) * 0.12;
}

export function brightnessToStored(value: number) {
  return (value / 0.12) * 100;
}

export function ratioFromStored(value: number) {
  return value / 100;
}

export function ratioToStored(value: number) {
  return value * 100;
}
