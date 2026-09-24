import { useInfiniteQuery } from "@tanstack/react-query";
import { Globe, KeyRound, LogIn, Package, Receipt, Server, Settings, ShieldCheck, Smartphone, Sparkles, UserRound, History, type LucideIcon } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { date, relative } from "@shared/format";
import { Button, Card, Empty, PageLoader } from "@shared/ui";
import type { Activity } from "../types";
import { PageHeader } from "../components";

const icons: Record<string, LucideIcon> = {
  login: LogIn, signup: Sparkles, user: UserRound, device: Smartphone, plan: Package, payment: Receipt,
  gate: Server, domain: Globe, settings: Settings, security: ShieldCheck, backup: History, setup: KeyRound,
};
export const activityIcon = (kind: string) => icons[kind] || Sparkles;

/** The server stores a message key and its arguments, separated by U+001F. */
export function activityText(detail: string, t: (k: string, v?: Record<string, string>) => string) {
  const [key, ...args] = detail.split("\x1f");
  const vars: Record<string, string> = {};
  args.forEach((a, i) => (vars[String(i)] = a));
  const out = t("act.m." + key, vars);
  return out === "act.m." + key ? detail.replaceAll("\x1f", " ") : out;
}

export function ActivityPage() {
  const { t, lang } = useI18n();
  const q = useInfiniteQuery({
    queryKey: ["activity"],
    queryFn: ({ pageParam }) => api<Activity[]>(`/activity?limit=50&before=${pageParam}`),
    initialPageParam: 0,
    getNextPageParam: (last) => (last.length === 50 ? last[last.length - 1].id : undefined),
  });
  if (q.isLoading) return <PageLoader />;
  const items = q.data?.pages.flat() || [];

  // Group by calendar day for a readable timeline.
  const groups: { day: string; items: Activity[] }[] = [];
  for (const a of items) {
    const day = date(a.at, lang);
    if (!groups.length || groups[groups.length - 1].day !== day) groups.push({ day, items: [] });
    groups[groups.length - 1].items.push(a);
  }

  return (
    <div>
      <PageHeader title={t("act.title")} sub={t("act.sub")} />
      {items.length === 0 ? (
        <Card><Empty icon={<History className="size-6" />} title={t("act.empty")} /></Card>
      ) : (
        <div className="space-y-6">
          {groups.map((g) => (
            <div key={g.day}>
              <div className="text-xs font-bold text-muted mb-2 px-1">{g.day}</div>
              <Card className="divide-y divide-line">
                {g.items.map((a) => {
                  const Icon = activityIcon(a.kind);
                  return (
                    <div key={a.id} className="flex items-center gap-3 px-4 py-3">
                      <span className="size-9 rounded-xl bg-surface-2 grid place-items-center shrink-0"><Icon className="size-4 text-primary" /></span>
                      <div className="flex-1 min-w-0 text-sm">
                        <b className="ltr">{a.actor}</b> <span className="text-muted">{activityText(a.detail, t)}</span>
                      </div>
                      <span className="text-xs text-faint whitespace-nowrap" title={date(a.at, lang, true)}>{relative(a.at, lang)}</span>
                    </div>
                  );
                })}
              </Card>
            </div>
          ))}
          {q.hasNextPage && (
            <div className="text-center">
              <Button variant="secondary" onClick={() => q.fetchNextPage()} loading={q.isFetchingNextPage}>{t("ui.loadmore")}</Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
