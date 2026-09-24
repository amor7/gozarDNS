import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { cn } from "./ui";

function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [w, setW] = useState(0);
  useEffect(() => {
    if (!ref.current) return;
    const ro = new ResizeObserver(([e]) => setW(e.contentRect.width));
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, []);
  return [ref, w] as const;
}

/** Monotone cubic path: smooth, never overshoots below zero. */
function monotonePath(pts: [number, number][]) {
  const n = pts.length;
  if (n === 0) return "";
  if (n === 1) return `M${pts[0][0]},${pts[0][1]}`;
  const dx: number[] = [], m: number[] = [], t: number[] = [];
  for (let i = 0; i < n - 1; i++) {
    dx[i] = pts[i + 1][0] - pts[i][0];
    m[i] = (pts[i + 1][1] - pts[i][1]) / (dx[i] || 1);
  }
  t[0] = m[0];
  t[n - 1] = m[n - 2];
  for (let i = 1; i < n - 1; i++) t[i] = m[i - 1] * m[i] <= 0 ? 0 : (3 * (dx[i - 1] + dx[i])) / ((2 * dx[i] + dx[i - 1]) / m[i - 1] + (dx[i] + 2 * dx[i - 1]) / m[i]);
  let d = `M${pts[0][0]},${pts[0][1]}`;
  for (let i = 0; i < n - 1; i++) {
    const h = dx[i] / 3;
    d += `C${pts[i][0] + h},${pts[i][1] + h * t[i]} ${pts[i + 1][0] - h},${pts[i + 1][1] - h * t[i + 1]} ${pts[i + 1][0]},${pts[i + 1][1]}`;
  }
  return d;
}

export type Series = { key: string; label: string; color: string };

export function AreaChart<T extends Record<string, number>>({ data, series, height = 220, formatY, formatX, className, stacked = false }: {
  data: T[]; series: Series[]; height?: number; formatY: (v: number) => string; formatX: (d: T) => string; className?: string; stacked?: boolean;
}) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);
  const uid = useId().replace(/:/g, "");
  const padL = 8, padR = 8, padT = 12, padB = 26;
  const w = Math.max(width, 10);
  const innerW = w - padL - padR, innerH = height - padT - padB;
  const totals = data.map((d) => (stacked ? series.reduce((s, x) => s + (d[x.key] || 0), 0) : Math.max(...series.map((x) => d[x.key] || 0))));
  const maxV = Math.max(1, ...totals) * 1.12;
  const x = (i: number) => padL + (data.length <= 1 ? innerW / 2 : (i / (data.length - 1)) * innerW);
  const y = (v: number) => padT + innerH - (v / maxV) * innerH;

  const layers = series.map((s, si) => {
    const pts: [number, number][] = data.map((d, i) => {
      const base = stacked ? series.slice(0, si).reduce((acc, p) => acc + (d[p.key] || 0), 0) : 0;
      return [x(i), y(base + (d[s.key] || 0))];
    });
    const basePts: [number, number][] = data.map((d, i) => {
      const base = stacked ? series.slice(0, si).reduce((acc, p) => acc + (d[p.key] || 0), 0) : 0;
      return [x(i), y(base)];
    });
    const line = monotonePath(pts);
    const area = line + "L" + basePts.slice().reverse().map((p) => p.join(",")).join("L") + "Z";
    return { s, line, area, pts };
  });

  const ticks = [0.25, 0.5, 0.75, 1].map((f) => maxV * f / 1.12);
  const labelEvery = Math.max(1, Math.ceil(data.length / Math.max(2, Math.floor(innerW / 70))));

  return (
    <div ref={ref} className={cn("relative select-none ltr", className)} style={{ height }}>
      {width > 0 && (
        <svg width={w} height={height} onMouseLeave={() => setHover(null)} onMouseMove={(e) => {
          const r = (e.currentTarget as SVGElement).getBoundingClientRect();
          const px = e.clientX - r.left;
          const i = Math.round(((px - padL) / innerW) * (data.length - 1));
          setHover(Math.min(data.length - 1, Math.max(0, i)));
        }}>
          <defs>
            {series.map((s) => (
              <linearGradient key={s.key} id={`${uid}-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0" stopColor={s.color} stopOpacity="0.32" />
                <stop offset="1" stopColor={s.color} stopOpacity="0" />
              </linearGradient>
            ))}
          </defs>
          {ticks.map((tk) => (
            <line key={tk} x1={padL} x2={w - padR} y1={y(tk)} y2={y(tk)} stroke="var(--line)" strokeDasharray="3 5" />
          ))}
          {layers.map((l) => (
            <g key={l.s.key}>
              <path d={l.area} fill={`url(#${uid}-${l.s.key})`} />
              <path d={l.line} fill="none" stroke={l.s.color} strokeWidth="2.2" strokeLinejoin="round" strokeLinecap="round" />
            </g>
          ))}
          {data.map((d, i) =>
            i % labelEvery === 0 ? (
              <text key={i} x={x(i)} y={height - 6} textAnchor="middle" fontSize="11" fill="var(--faint)">
                {formatX(d)}
              </text>
            ) : null,
          )}
          {hover !== null && (
            <g>
              <line x1={x(hover)} x2={x(hover)} y1={padT} y2={padT + innerH} stroke="var(--line-strong)" />
              {layers.map((l) => (
                <circle key={l.s.key} cx={l.pts[hover][0]} cy={l.pts[hover][1]} r="4.5" fill="var(--surface)" stroke={l.s.color} strokeWidth="2.5" />
              ))}
            </g>
          )}
        </svg>
      )}
      {hover !== null && data[hover] && (
        <div
          className="absolute top-1 pointer-events-none glass rounded-xl shadow-pop px-3 py-2 text-xs z-10 min-w-32"
          style={{ left: Math.min(Math.max(x(hover) - 70, 0), w - 150) }}
        >
          <div className="text-muted mb-1">{formatX(data[hover])}</div>
          {series.map((s) => (
            <div key={s.key} className="flex items-center justify-between gap-4">
              <span className="flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: s.color }} />{s.label}</span>
              <b className="num">{formatY(data[hover][s.key] || 0)}</b>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function BarChart({ data, height = 160, format, label, className, highlightLast = true }: {
  data: { key: string; value: number }[]; height?: number; format: (v: number) => string; label: (k: string) => string; className?: string; highlightLast?: boolean;
}) {
  const [hover, setHover] = useState<number | null>(null);
  const max = Math.max(1, ...data.map((d) => d.value));
  const labelEvery = Math.max(1, Math.ceil(data.length / 5));
  return (
    <div className={cn("relative ltr", className)}>
      <div className="flex items-end gap-[3px] sm:gap-1" style={{ height }} onMouseLeave={() => setHover(null)}>
        {data.map((d, i) => {
          const h = (d.value / max) * 100;
          const last = highlightLast && i === data.length - 1;
          return (
            <div key={d.key} className="flex-1 h-full flex flex-col justify-end cursor-default" onMouseEnter={() => setHover(i)}>
              <div
                className={cn(
                  "w-full rounded-t-md transition-all duration-500",
                  hover === i || last ? "gradient-bg" : "bg-primary/25 dark:bg-primary/30",
                )}
                style={{ height: `${Math.max(h, d.value > 0 ? 3 : 1.5)}%` }}
              />
            </div>
          );
        })}
      </div>
      <div className="flex gap-[3px] sm:gap-1 mt-2">
        {data.map((d, i) => (
          <div key={d.key} className="flex-1 relative h-4">
            {i % labelEvery === 0 && <span className="absolute start-1/2 -translate-x-1/2 text-[10px] text-faint whitespace-nowrap">{label(d.key)}</span>}
          </div>
        ))}
      </div>
      {hover !== null && (
        <div className="absolute -top-2 pointer-events-none glass rounded-xl shadow-pop px-3 py-1.5 text-xs z-10 whitespace-nowrap"
          style={{ left: `clamp(0px, calc(${((hover + 0.5) / data.length) * 100}% - 60px), calc(100% - 120px))` }}>
          <span className="text-muted">{label(data[hover].key)}</span> · <b className="num">{format(data[hover].value)}</b>
        </div>
      )}
    </div>
  );
}

export function Sparkline({ values, color = "var(--primary)", height = 36, width = 120 }: { values: number[]; color?: string; height?: number; width?: number }) {
  const uid = useId().replace(/:/g, "");
  if (values.length < 2) return <svg width={width} height={height} />;
  const max = Math.max(1, ...values);
  const pts: [number, number][] = values.map((v, i) => [(i / (values.length - 1)) * width, height - 2 - (v / max) * (height - 4)]);
  const line = monotonePath(pts);
  return (
    <svg width={width} height={height} className="ltr overflow-visible">
      <defs>
        <linearGradient id={uid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor={color} stopOpacity="0.3" />
          <stop offset="1" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={`${line}L${width},${height}L0,${height}Z`} fill={`url(#${uid})`} />
      <path d={line} fill="none" stroke={color} strokeWidth="1.8" />
    </svg>
  );
}

export function Legend({ items }: { items: { label: ReactNode; color: string }[] }) {
  return (
    <div className="flex flex-wrap items-center gap-4 text-xs text-muted">
      {items.map((it, i) => (
        <span key={i} className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-full" style={{ background: it.color }} />
          {it.label}
        </span>
      ))}
    </div>
  );
}
