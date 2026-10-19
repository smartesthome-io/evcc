const nf = (digits: number) =>
  new Intl.NumberFormat(undefined, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  });

export function fmtPower(watt: number | undefined): string {
  const w = Number.isFinite(watt) ? (watt as number) : 0;
  if (Math.abs(w) >= 1000) return `${nf(1).format(w / 1000)} kW`;
  return `${nf(0).format(w)} W`;
}

export function fmtEnergy(kwh: number | undefined): string {
  const v = Number.isFinite(kwh) ? (kwh as number) : 0;
  return `${nf(v >= 100 ? 0 : v >= 10 ? 1 : 2).format(v)} kWh`;
}

export function fmtPercent(value: number | undefined): string {
  const v = Number.isFinite(value) ? (value as number) : 0;
  return `${nf(0).format(v)} %`;
}

export function fmtDuration(seconds: number | undefined): string {
  if (!Number.isFinite(seconds) || !seconds || seconds <= 0) return "";
  const h = Math.floor(seconds / 3600);
  const m = Math.round((seconds % 3600) / 60);
  return h > 0 ? `${h}:${String(m).padStart(2, "0")} h` : `${m} min`;
}
