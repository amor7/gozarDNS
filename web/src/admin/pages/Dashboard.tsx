import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { ArrowUpRight, CalendarClock, Download, Receipt, Server, Users as UsersIcon, Wallet, Wifi } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { bytes, bytesParts, daysLeft, num, rate, relative, shortDate, time, uptime } from "@shared/format";
import { Avatar, Badge, Button, Card, CardHeader, Empty, PageLoader, Stat } from "@shared/ui";
import { AreaChart, BarChart, Legend } from "@shared/charts";
import type { Gate, Overview, Sample } from "../types";
import { Meter, PageHeader } from "../components";
import { activityIcon, activityText } from "./Activity";

export function Dashboard() {
  const { t, lang } = useI18n();
  const q = useQuery({ queryKey: ["overview"], queryFn: () => api<Overview>("/overview"), refetchInterval: 30_000 });
  if (!q.data) return <PageLoader />;
  const d = q.data;
  const tt = d.totals;
  const [todayV, todayU] = bytesParts(tt.traffic_today, lang);

  return (
    <div className="space-y-5">
      <PageHeader title={t("dash.title")} sub={t("dash.hello")} />

      {tt.pending_payments > 0 && (
        <Link to="/payments" className="block animate-in">
          <div className="card p-4 flex items-center gap-3 border-warning/40 bg-warning-soft hover:brightness-105 transition">
            <div className="size-10 rounded-xl bg-warning text-white grid place-items-center"><Receipt className="size-5" /></div>
            <div className="flex-1 font-bold">{t("dash.pending", { n: num(tt.pending_payments, lang) })}</div>
            <Button size="sm" variant="secondary">{t("dash.pending.cta")}</Button>
          </div>
        </Link>
      )}

      {d.gates.length === 0 && (
        <Card><Empty icon={<Server className="size-6" />} title={t("dash.nogate.t")} sub={t("dash.nogate.s")} action={<Link to="/gates"><Button>{t("dash.nogate.cta")}</Button></Link>} /></Card>
      )}

      <div className="grid grid-cols-2 xl:grid-cols-4 gap-3 sm:gap-4">
        <Stat label={t("dash.users")} value={num(tt.users, lang)} icon={<UsersIcon className="size-[18px]" />} foot={t("dash.users.foot", { n: num(tt.active, lang) })} />
        <Stat label={t("dash.online")} value={num(tt.online_now, lang)} icon={<Wifi className="size-[18px]" />} tone="success" foot={t("dash.online.foot", { n: num(tt.devices, lang) })} />
        <Stat label={t("dash.today")} value={todayV} unit={todayU} icon={<Download className="size-[18px]" />} tone="info" foot={t("dash.today.foot", { v: bytes(tt.traffic_month, lang) })} />
        <Stat label={t("dash.revenue")} value={num(tt.revenue_month, lang)} icon={<Wallet className="size-[18px]" />} tone="warning" foot={t("dash.revenue.foot", { n: num(tt.pending_payments, lang) })} />
      </div>

      <div className="grid xl:grid-cols-3 gap-4">
        <Card className="xl:col-span-2">
          <CardHeader title={t("dash.traffic")} sub={t("dash.traffic.sub")}
            action={<Legend items={[{ label: t("dash.down"), color: "var(--primary)" }, { label: t("dash.up"), color: "var(--primary-2)" }]} />} />
          <div className="px-3 pb-4">
            <AreaChart
              data={d.traffic.map((p) => ({ ts: p.ts, down: p.down, up: p.up }))}
              series={[{ key: "down", label: t("dash.down"), color: "var(--primary)" }, { key: "up", label: t("dash.up"), color: "var(--primary-2)" }]}
              formatY={(v) => bytes(v, lang)}
              formatX={(p) => time(p.ts, lang)}
              height={250}
            />
          </div>
        </Card>
        <Card>
          <CardHeader title={t("dash.top")} sub={t("dash.top.sub")} />
          <div className="px-3 pb-3">
            {d.top.map((u, i) => {
              const max = d.top[0]?.bytes || 1;
              return (
                <Link key={u.id} to={`/users/${u.id}`} className="flex items-center gap-3 p-2.5 rounded-xl hover:bg-surface-2">
                  <span className="w-4 text-xs text-faint font-bold num">{num(i + 1, lang)}</span>
                  <Avatar name={u.username} size={32} />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-semibold truncate ltr text-start">{u.username}</div>
                    <div className="h-1 rounded-full bg-surface-3 mt-1.5 overflow-hidden">
                      <div className="h-full gradient-bg rounded-full" style={{ width: `${(u.bytes / max) * 100}%` }} />
                    </div>
                  </div>
                  <span className="text-xs font-bold num text-muted">{bytes(u.bytes, lang)}</span>
                </Link>
              );
            })}
          </div>
        </Card>
      </div>

      <div>
        <h2 className="font-bold mb-3">{t("dash.nodes")}</h2>
        <div className="grid md:grid-cols-2 xl:grid-cols-3 gap-4">
          <NodeCard name={t("dash.hub")} online sample={d.hub.sample} conns={d.hub.conns} tag="hub" />
          {d.gates.map((g) => (
            <GateNode key={g.id} g={g} />
          ))}
        </div>
      </div>

      <div className="grid xl:grid-cols-3 gap-4">
        <Card>
          <CardHeader title={t("dash.daily")} />
          <div className="px-5 pb-5 pt-2">
            <BarChart data={d.daily.map((x) => ({ key: x.day, value: x.bytes }))} height={140} format={(v) => bytes(v, lang)} label={(k) => shortDate(k, lang)} />
          </div>
        </Card>
        <Card>
          <CardHeader title={t("dash.expiring")} icon={<CalendarClock className="size-[18px]" />} />
          <div className="px-3 pb-3">
            {d.expiring.length === 0 && <div className="text-sm text-muted text-center py-8 px-4">{t("dash.expiring.none")}</div>}
            {d.expiring.map((u) => {
              const days = daysLeft(u.expires_at) ?? 0;
              return (
                <Link key={u.id} to={`/users/${u.id}`} className="flex items-center gap-3 p-2.5 rounded-xl hover:bg-surface-2">
                  <Avatar name={u.username} size={32} />
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-semibold truncate">{u.display_name || u.username}</div>
                    <div className="text-xs text-muted">{u.plan_name}</div>
                  </div>
                  <Badge tone={days <= 1 ? "danger" : "warning"}>{t("users.days", { n: num(days, lang) })}</Badge>
                </Link>
              );
            })}
          </div>
        </Card>
        <Card>
          <CardHeader title={t("dash.activity")} action={<Link to="/activity" className="text-primary"><ArrowUpRight className="size-5" /></Link>} />
          <div className="px-5 pb-4 space-y-3">
            {d.activity.map((a) => {
              const Icon = activityIcon(a.kind);
              return (
                <div key={a.id} className="flex gap-3 text-sm">
                  <span className="size-8 rounded-lg bg-surface-2 grid place-items-center shrink-0"><Icon className="size-4 text-muted" /></span>
                  <div className="min-w-0">
                    <div className="truncate"><b className="ltr">{a.actor}</b> <span className="text-muted">{activityText(a.detail, t)}</span></div>
                    <div className="text-[11px] text-faint">{relative(a.at, lang)}</div>
                  </div>
                </div>
              );
            })}
          </div>
        </Card>
      </div>
    </div>
  );
}

function GateNode({ g }: { g: Gate }) {
  return <NodeCard name={g.name} online={g.online} sample={g.stats?.host} conns={g.stats?.stats?.open_conns} ip={g.public_ip} seen={g.last_seen_at} tag="gate" />;
}

function NodeCard({ name, online, sample, conns, ip, seen, tag }: { name: string; online: boolean; sample?: Sample; conns?: number; ip?: string; seen?: number | null; tag: "hub" | "gate" }) {
  const { t, lang } = useI18n();
  const s = sample;
  const pct = (a?: number, b?: number) => (a && b ? (a / b) * 100 : 0);
  return (
    <Card className="p-5 animate-in">
      <div className="flex items-center gap-3">
        <div className={`size-10 rounded-xl grid place-items-center ${tag === "hub" ? "gradient-bg text-white" : "bg-surface-2 text-primary"}`}>
          <Server className="size-5" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="font-bold truncate ltr text-start">{name}</div>
          <div className="text-xs text-muted ltr text-start num">{ip || (seen ? relative(seen, lang) : "")}</div>
        </div>
        <Badge tone={online ? "success" : "danger"} dot>{online ? t("dash.online.badge") : t("dash.offline")}</Badge>
      </div>
      {s ? (
        <div className="mt-5 space-y-3.5">
          <Meter label={t("dash.cpu")} value={`${num(s.cpu, lang, 1)}٪`.replace("٪", lang === "fa" ? "٪" : "%")} pct={s.cpu} />
          <Meter label={t("dash.ram")} value={`${bytes(s.mem_used, lang)} / ${bytes(s.mem_total, lang)}`} pct={pct(s.mem_used, s.mem_total)} />
          <Meter label={t("dash.disk")} value={`${bytes(s.disk_used, lang)} / ${bytes(s.disk_total, lang)}`} pct={pct(s.disk_used, s.disk_total)} />
          <div className="grid grid-cols-3 gap-2 pt-1 text-center">
            <Mini label={t("dash.net")} value={`↓ ${rate(s.rx_bps, lang)}`} />
            <Mini label={t("dash.conns")} value={num(conns ?? s.conns ?? 0, lang)} />
            <Mini label={t("dash.uptime")} value={uptime(s.uptime, lang)} />
          </div>
        </div>
      ) : (
        <div className="mt-5 text-sm text-muted">{t("gates.never")}</div>
      )}
    </Card>
  );
}

function Mini({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl bg-surface-2 py-2 px-1">
      <div className="text-[10px] text-faint">{label}</div>
      <div className="text-xs font-bold mt-0.5 num truncate">{value}</div>
    </div>
  );
}
