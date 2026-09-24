import type { ReactNode } from "react";
import { useI18n } from "@shared/i18n";
import { Badge } from "@shared/ui";
import type { State } from "./types";

export function PageHeader({ title, sub, actions }: { title: string; sub?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-6 animate-in">
      <div>
        <h1 className="text-2xl sm:text-[28px] font-extrabold tracking-tight">{title}</h1>
        {sub && <p className="text-muted mt-1 text-sm sm:text-[15px]">{sub}</p>}
      </div>
      {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
    </div>
  );
}

const tones = { active: "success", no_plan: "neutral", expired: "danger", over_quota: "warning", disabled: "danger" } as const;

export function StateBadge({ state }: { state: State }) {
  const { t } = useI18n();
  return (
    <Badge tone={tones[state]} dot>
      {t("state." + state)}
    </Badge>
  );
}

/** A labelled value in a detail panel. */
export function KV({ k, v }: { k: ReactNode; v: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-2.5 border-b border-line last:border-0 text-sm">
      <span className="text-muted">{k}</span>
      <span className="font-semibold text-end">{v}</span>
    </div>
  );
}

export function Meter({ label, value, pct, sub }: { label: string; value: string; pct: number; sub?: string }) {
  const tone = pct >= 90 ? "bg-danger" : pct >= 75 ? "bg-warning" : "gradient-bg";
  return (
    <div>
      <div className="flex items-baseline justify-between text-xs mb-1.5">
        <span className="text-muted">{label}</span>
        <span className="font-bold num">{value}</span>
      </div>
      <div className="h-1.5 rounded-full bg-surface-3 overflow-hidden">
        <div className={`h-full rounded-full ${tone}`} style={{ width: `${Math.min(100, Math.max(pct, 1))}%` }} />
      </div>
      {sub && <div className="text-[11px] text-faint mt-1 num">{sub}</div>}
    </div>
  );
}
