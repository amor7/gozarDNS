import { useState } from "react";
import { Link } from "react-router";
import { Activity, BarChart3, CalendarClock, CircleAlert, CircleCheck, Gauge, Headset, Laptop, MonitorSmartphone, Pencil, Plus, Radar, RefreshCw, ShieldAlert, Trash2, TrendingUp, TriangleAlert, Wifi, Zap } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { bytes, bytesParts, date, daysLeft, num, relative, shortDate, speed } from "@shared/format";
import { Badge, Button, Card, CardHeader, CopyButton, cn, Field, Input, PageLoader, Ring, ServiceDot } from "@shared/ui";
import { BarChart } from "@shared/charts";
import { Modal, useConfirm, useToast } from "@shared/overlay";
import { useMe, useRefresh } from "../data";
import { ProbeResult, useProbe } from "../probe";
import type { Me } from "../types";

const stateTone = { active: "success", no_plan: "neutral", expired: "danger", over_quota: "danger", disabled: "danger" } as const;

export function Home() {
  const me = useMe().data;
  const { t } = useI18n();
  if (!me) return <PageLoader />;
  const u = me.user;
  return (
    <div className="space-y-4 animate-in">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-extrabold">{t("home.hi", { name: u.display_name || u.username })}</h1>
          {me.plan && <p className="text-muted text-sm mt-0.5">{me.plan.name}</p>}
        </div>
        <Badge tone={stateTone[u.state]} dot>
          {t("home.state." + u.state)}
        </Badge>
      </div>

      <Alerts me={me} />

      <div className="grid lg:grid-cols-5 gap-4">
        <UsageCard me={me} />
        <div className="lg:col-span-2 space-y-4">
          <ConnectionCard me={me} />
          <DnsCard dns={me.dns} />
        </div>
      </div>

      <div className="grid lg:grid-cols-5 gap-4">
        <UsageChart me={me} />
        <Devices me={me} />
      </div>

      {me.plan && me.plan.services.length > 0 && <Services me={me} />}
    </div>
  );
}

function Alerts({ me }: { me: Me }) {
  const { t, lang } = useI18n();
  const u = me.user;
  const days = daysLeft(u.expires_at);
  const pct = u.quota_bytes > 0 ? Math.round((u.used_bytes / u.quota_bytes) * 100) : 0;
  let a: { tone: "danger" | "warning" | "info"; title: string; sub: string; cta: string } | null = null;
  if (u.state !== "active") {
    a = { tone: u.state === "no_plan" ? "info" : "danger", title: t(`home.alert.${u.state}.t`), sub: t(`home.alert.${u.state}.s`), cta: u.state === "no_plan" ? t("home.choose") : t("home.renew") };
  } else if (pct >= 80) {
    a = { tone: "warning", title: t("home.alert.low.t"), sub: t("home.alert.low.s", { pct: num(pct, lang) }), cta: t("home.renew") };
  } else if (days !== null && days <= 3) {
    a = { tone: "warning", title: t("home.alert.soon.t"), sub: t("home.alert.soon.s", { days: num(days, lang) }), cta: t("home.renew") };
  }
  if (!a) return null;
  const tones = { danger: "bg-danger-soft text-danger", warning: "bg-warning-soft text-warning", info: "bg-primary-soft text-primary" };
  const Icon = a.tone === "info" ? Zap : a.tone === "danger" ? ShieldAlert : TriangleAlert;
  return (
    <Card className="p-4 sm:p-5 flex flex-col sm:flex-row sm:items-center gap-4">
      <div className={cn("size-11 rounded-2xl grid place-items-center shrink-0", tones[a.tone])}>
        <Icon className="size-5" />
      </div>
      <div className="flex-1">
        <div className="font-bold">{a.title}</div>
        <div className="text-sm text-muted mt-0.5">{a.sub}</div>
      </div>
      <div className="flex gap-2 w-full sm:w-auto">
        {u.state !== "disabled" && (
          <Link to="/plans" className="flex-1">
            <Button className="w-full">{a.cta}</Button>
          </Link>
        )}
        {a.tone !== "warning" && (
          <Link to="/support" className="flex-1">
            <Button variant="secondary" className="w-full" icon={<Headset className="size-4" />}>{t("sup.contact")}</Button>
          </Link>
        )}
      </div>
    </Card>
  );
}

function UsageCard({ me }: { me: Me }) {
  const { t, lang } = useI18n();
  const u = me.user;
  const unlimited = u.quota_bytes === 0;
  const left = Math.max(0, u.quota_bytes - u.used_bytes);
  const [v, unit] = bytesParts(unlimited ? u.used_bytes : left, lang);
  const days = daysLeft(u.expires_at);
  const periodDays = u.expires_at && u.period_start ? Math.max(1, Math.round((u.expires_at - u.period_start) / 86400)) : 0;

  return (
    <Card className="lg:col-span-3 p-5 sm:p-6 relative overflow-hidden flex flex-col">
      <div className="absolute -top-24 -end-24 size-64 rounded-full gradient-bg opacity-[0.08] blur-2xl pointer-events-none" />
      <div className="flex-1 flex flex-col sm:flex-row items-center gap-6 sm:gap-8">
        <Ring value={unlimited ? 0 : u.used_bytes} max={unlimited ? 1 : u.quota_bytes} size={188} stroke={15}>
          <div>
            <div className="text-[34px] font-black tracking-tight num leading-none">{v}</div>
            <div className="text-sm text-muted mt-1">{unit}</div>
            <div className="text-xs text-faint mt-1.5">{unlimited ? t("home.used") : t("home.left")}</div>
          </div>
        </Ring>
        <div className="flex-1 w-full grid grid-cols-2 gap-3">
          <MiniStat icon={<Gauge className="size-4" />} label={t("home.used")} value={bytes(u.used_bytes, lang)} sub={unlimited ? t("home.unlimited") : t("home.of", { total: bytes(u.quota_bytes, lang) })} />
          <MiniStat
            icon={<CalendarClock className="size-4" />}
            label={t("home.days")}
            value={days === null ? "∞" : num(days, lang)}
            sub={u.expires_at ? date(u.expires_at, lang) : t("home.days.none")}
            progress={days !== null && periodDays ? 1 - days / periodDays : undefined}
          />
          <MiniStat icon={<Zap className="size-4" />} label={t("home.speed")} value={speed(u.speed_kbps, lang)} />
          <MiniStat icon={<MonitorSmartphone className="size-4" />} label={t("home.devices")} value={t("dev.count", { n: num(me.devices.length, lang), max: num(u.max_devices, lang) })} />
        </div>
      </div>
      <UsagePace me={me} />
    </Card>
  );
}

/** Recent usage under the ring: today, the last week and its daily
 *  average. Fills the card's lower half on wide screens, where the column
 *  beside it is taller. The usage series always ends with today. */
function UsagePace({ me }: { me: Me }) {
  const { t, lang } = useI18n();
  const today = me.usage.at(-1)?.bytes ?? 0;
  const week = me.usage.slice(-7).reduce((s, d) => s + d.bytes, 0);
  const items = [
    { icon: <Activity className="size-4" />, label: t("home.pace.today"), value: bytes(today, lang) },
    { icon: <BarChart3 className="size-4" />, label: t("home.pace.week"), value: bytes(week, lang) },
    { icon: <TrendingUp className="size-4" />, label: t("home.pace.avg"), value: bytes(week / 7, lang) },
  ];
  return (
    <div className="mt-6 pt-5 border-t border-line flex flex-col sm:flex-row sm:items-center gap-4">
      <div className="flex-1 grid grid-cols-3 gap-3">
        {items.map((x) => (
          <div key={x.label} className="min-w-0">
            <div className="flex items-center gap-1.5 text-xs text-muted font-medium">
              <span className="text-primary">{x.icon}</span>
              <span className="leading-tight">{x.label}</span>
            </div>
            <div className="font-bold mt-1 num truncate">{x.value}</div>
          </div>
        ))}
      </div>
      {me.plan && (
        <Link to="/plans" className="shrink-0">
          <Button variant="soft" className="w-full sm:w-auto" icon={<RefreshCw className="size-4" />}>{t("home.renew")}</Button>
        </Link>
      )}
    </div>
  );
}

function MiniStat({ icon, label, value, sub, progress }: { icon: React.ReactNode; label: string; value: string; sub?: string; progress?: number }) {
  return (
    <div className="rounded-2xl bg-surface-2 border border-line p-3.5">
      <div className="flex items-center gap-1.5 text-xs text-muted font-medium">
        <span className="text-primary">{icon}</span>
        {label}
      </div>
      <div className="font-extrabold text-lg mt-1.5 num">{value}</div>
      {sub && <div className="text-[11px] text-faint mt-0.5 truncate">{sub}</div>}
      {progress !== undefined && (
        <div className="h-1 rounded-full bg-surface-3 mt-2 overflow-hidden">
          <div className="h-full gradient-bg rounded-full" style={{ width: `${Math.min(100, Math.max(0, progress * 100))}%` }} />
        </div>
      )}
    </div>
  );
}

function ConnectionCard({ me }: { me: Me }) {
  const { t } = useI18n();
  const meQuery = useMe();
  const toast = useToast();
  const refresh = useRefresh();
  const [open, setOpen] = useState(false);
  const [label, setLabel] = useState("");
  const [replace, setReplace] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const full = me.devices.length >= me.user.max_devices;

  const register = async () => {
    setBusy(true);
    try {
      await api("/devices", { body: { label, replace: replace ?? 0 } });
      toast.success(t("conn.registered"));
      setOpen(false);
      setLabel("");
      setReplace(null);
      refresh();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  const probe = useProbe();
  const recheck = async () => {
    const before = me.current_ip;
    const r = await meQuery.refetch();
    if (r.data && r.data.current_ip !== before) toast.info(t("conn.changed", { ip: r.data.current_ip }));
  };

  let tone: "success" | "warning" | "danger" = "warning";
  let icon = <Wifi className="size-5" />;
  let title = t("conn.new");
  let sub: string | null = t("conn.new.sub");
  if (me.ipv6) {
    [icon, title, sub] = [<CircleAlert className="size-5" />, t("conn.v6"), t("conn.v6.sub")];
  } else if (me.this_device) {
    [tone, icon, title, sub] = ["success", <CircleCheck className="size-5" />, t("conn.ok"), null];
  } else if (me.ip_taken) {
    [tone, icon, title, sub] = ["danger", <ShieldAlert className="size-5" />, t("conn.taken"), t("conn.taken.sub")];
  }
  const tones = { success: "bg-success-soft text-success", warning: "bg-warning-soft text-warning", danger: "bg-danger-soft text-danger" };

  return (
    <Card className="p-5">
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs font-semibold text-muted">{t("conn.title")}</span>
        <button onClick={recheck} disabled={meQuery.isFetching} title={t("conn.refresh")} aria-label={t("conn.refresh")}
          className="h-8 px-3 rounded-full text-xs font-bold bg-primary-soft text-primary border border-primary/25 hover:brightness-110 active:scale-[0.97] transition inline-flex items-center gap-1.5 disabled:opacity-60">
          <RefreshCw className={cn("size-3.5", meQuery.isFetching && "animate-spin")} />
          {t("conn.refresh.short")}
        </button>
      </div>

      <div className="flex gap-3 items-start">
        <div className={cn("size-10 rounded-xl grid place-items-center shrink-0", tones[tone])}>{icon}</div>
        <div className="min-w-0 pt-0.5">
          <div className="font-bold text-[15px] leading-snug">{title}</div>
          {sub && <div className="text-[13px] text-muted mt-1 leading-relaxed">{sub}</div>}
        </div>
      </div>

      {!me.ipv6 && (
        <div className="mt-4 flex items-center justify-between gap-3 rounded-xl bg-surface-2 px-3.5 h-11">
          <span className="text-[13px] text-muted">{t("conn.ip")}</span>
          <span className="ltr num font-bold tracking-wide">{me.current_ip}</span>
        </div>
      )}

      {me.this_device ? (
        <div className="mt-3 space-y-3">
          <Button variant="secondary" block icon={<Radar className={cn("size-4", probe.state === "running" && "animate-spin [animation-duration:2s]")} />}
            loading={probe.state === "running"} onClick={probe.run}>
            {probe.state === "running" ? t("setup.test.running") : t("conn.test")}
          </Button>
          {probe.state === "done" && <ProbeResult res={probe.res} />}
        </div>
      ) : !me.ip_taken && !me.ipv6 ? (
        <Button block className="mt-3" icon={<Plus className="size-4" />} onClick={() => setOpen(true)} disabled={me.user.state === "disabled"}>
          {t("conn.register")}
        </Button>
      ) : null}

      <Modal
        open={open}
        onClose={() => setOpen(false)}
        title={full ? t("conn.replace.t") : t("conn.register")}
        sub={full ? t("conn.replace.s") : undefined}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>{t("ui.cancel")}</Button>
            <Button onClick={register} loading={busy} disabled={full && !replace}>
              {full ? t("conn.replace") : t("conn.register")}
            </Button>
          </>
        }
      >
        {full && (
          <div className="space-y-2 mb-4">
            {me.devices.map((d) => (
              <button key={d.id} type="button" onClick={() => setReplace(d.id)}
                className={cn("w-full flex items-center gap-3 p-3 rounded-2xl border text-start transition-colors", replace === d.id ? "border-primary bg-primary-soft" : "border-line hover:bg-surface-2")}>
                <Laptop className="size-5 text-muted" />
                <div className="flex-1 min-w-0">
                  <div className="font-semibold text-sm truncate">{d.label || t("dev.unnamed")}</div>
                  <div className="text-xs text-muted ltr text-start num">{d.ip}</div>
                </div>
              </button>
            ))}
          </div>
        )}
        <Field label={t("conn.label")}>
          <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("conn.label.ph")} maxLength={40} />
        </Field>
        <div className="mt-3 text-xs text-muted">
          IP: <span className="ltr num font-semibold text-fg">{me.current_ip}</span>
        </div>
      </Modal>
    </Card>
  );
}

function DnsCard({ dns }: { dns: string }) {
  const { t } = useI18n();
  return (
    <Card className="p-5 relative overflow-hidden">
      <div className="text-xs font-semibold text-muted">{t("dns.title")}</div>
      <div className="mt-3 rounded-2xl border border-dashed border-line-strong bg-surface-2 py-4 text-center">
        <div className="ltr num text-[28px] font-black tracking-wide gradient-text">{dns || "—"}</div>
      </div>
      <p className="text-[13px] text-muted mt-3 leading-relaxed">{t("dns.sub")}</p>
      <div className="grid grid-cols-2 gap-2 mt-4">
        <CopyButton text={dns} size="md" />
        <Link to="/setup">
          <Button variant="soft" block>{t("dns.how")}</Button>
        </Link>
      </div>
    </Card>
  );
}

function UsageChart({ me }: { me: Me }) {
  const { t, lang } = useI18n();
  const data = me.usage.slice(-14).map((d) => ({ key: d.day, value: d.bytes }));
  const total = data.reduce((s, d) => s + d.value, 0);
  return (
    <Card className="lg:col-span-3">
      <CardHeader title={t("usage.title")} sub={t("usage.sub")} action={<Badge tone="primary">{t("usage.total", { v: bytes(total, lang) })}</Badge>} />
      <div className="px-5 pb-5 pt-2">
        <BarChart data={data} height={150} format={(v) => bytes(v, lang)} label={(k) => shortDate(k, lang)} />
      </div>
    </Card>
  );
}

function Devices({ me }: { me: Me }) {
  const { t, lang } = useI18n();
  const confirm = useConfirm();
  const toast = useToast();
  const refresh = useRefresh();
  const [editing, setEditing] = useState<{ id: number; label: string } | null>(null);
  const [manual, setManual] = useState(false);

  const remove = async (id: number, ip: string) => {
    if (!(await confirm({ title: t("dev.remove.q"), body: t("dev.remove.s", { ip }), confirm: t("dev.remove"), danger: true }))) return;
    try {
      await api(`/devices/${id}`, { method: "DELETE" });
      toast.success(t("dev.removed"));
      refresh();
    } catch (e) {
      toast.error(e);
    }
  };
  const rename = async () => {
    if (!editing) return;
    try {
      await api(`/devices/${editing.id}`, { method: "PATCH", body: { label: editing.label } });
      setEditing(null);
      refresh();
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <Card className="lg:col-span-2">
      <CardHeader title={t("dev.title")} action={<Badge>{t("dev.count", { n: num(me.devices.length, lang), max: num(me.user.max_devices, lang) })}</Badge>} />
      <div className="px-5 -mt-1 mb-2">
        <button onClick={() => setManual(true)} className="text-sm text-primary font-semibold inline-flex items-center gap-1.5 hover:underline">
          <Plus className="size-4" />
          {t("dev.manual")}
        </button>
      </div>
      {manual && <ManualIP me={me} onClose={() => setManual(false)} />}
      <div className="px-3 pb-3">
        {me.devices.length === 0 && <div className="text-sm text-muted px-2 py-6 text-center">{t("dev.none")}</div>}
        {me.devices.map((d) => (
          <div key={d.id} className="flex items-center gap-3 p-2.5 rounded-2xl hover:bg-surface-2 group">
            <div className={cn("size-10 rounded-xl grid place-items-center", d.id === me.this_device ? "gradient-bg text-white" : "bg-surface-3 text-muted")}>
              <MonitorSmartphone className="size-[18px]" />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <span className="font-semibold text-sm truncate">{d.label || t("dev.unnamed")}</span>
                {d.id === me.this_device && <Badge tone="primary" className="h-5 text-[10px]">{t("dev.this")}</Badge>}
              </div>
              <div className="text-xs text-muted mt-0.5 flex items-center gap-2">
                <span className="ltr num">{d.ip}</span>·<span>{d.last_seen_at ? t("dev.seen", { when: relative(d.last_seen_at, lang) }) : t("dev.never")}</span>
              </div>
            </div>
            <button className="size-8 rounded-lg grid place-items-center text-faint hover:text-fg hover:bg-surface-3 sm:opacity-0 group-hover:opacity-100 transition" onClick={() => setEditing({ id: d.id, label: d.label })} aria-label={t("dev.rename")}>
              <Pencil className="size-4" />
            </button>
            <button className="size-8 rounded-lg grid place-items-center text-faint hover:text-danger hover:bg-danger-soft sm:opacity-0 group-hover:opacity-100 transition" onClick={() => remove(d.id, d.ip)} aria-label={t("dev.remove")}>
              <Trash2 className="size-4" />
            </button>
          </div>
        ))}
      </div>
      <Modal open={!!editing} onClose={() => setEditing(null)} title={t("dev.rename")} size="sm"
        footer={<><Button variant="secondary" onClick={() => setEditing(null)}>{t("ui.cancel")}</Button><Button onClick={rename}>{t("ui.save")}</Button></>}>
        <Input value={editing?.label ?? ""} onChange={(e) => editing && setEditing({ ...editing, label: e.target.value })} placeholder={t("conn.label.ph")} maxLength={40} />
      </Modal>
    </Card>
  );
}

function Services({ me }: { me: Me }) {
  const { t, lang } = useI18n();
  const [all, setAll] = useState(false);
  const list = me.plan!.services;
  const shown = all ? list : list.slice(0, 10);
  return (
    <Card>
      <CardHeader title={t("svc.title")} action={<Badge>{num(list.length, lang)}</Badge>} />
      <div className="px-5 pb-5 flex flex-wrap gap-2">
        {list.length > shown.length && (
          <button onClick={() => setAll(true)} className="order-last h-10 px-3.5 rounded-xl bg-primary-soft text-primary text-sm font-semibold">
            {t("svc.more", { n: num(list.length - shown.length, lang) })}
          </button>
        )}
        {shown.map((s) => (
          <span key={s.id} className="inline-flex items-center gap-2 h-10 ps-1.5 pe-3.5 rounded-xl bg-surface-2 border border-line text-sm font-semibold">
            <ServiceDot name={s.name.en} color={s.color} size={28} />
            {s.name[lang]}
            {s.heavy && <span className="text-[10px] text-primary font-bold">{t("svc.heavy")}</span>}
          </span>
        ))}
      </div>
    </Card>
  );
}

/** Register a network other than the one this page is open on. */
function ManualIP({ me, onClose }: { me: Me; onClose: () => void }) {
  const { t } = useI18n();
  const toast = useToast();
  const refresh = useRefresh();
  const [ip, setIp] = useState("");
  const [label, setLabel] = useState("");
  const [replace, setReplace] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const full = me.devices.length >= me.user.max_devices;
  const submit = async () => {
    setBusy(true);
    try {
      await api("/devices", { body: { ip: ip.trim(), label, replace: replace ?? 0 } });
      toast.success(t("dev.manual.added"));
      refresh();
      onClose();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={t("dev.manual.t")} sub={t("dev.manual.s")}
      footer={<><Button variant="secondary" onClick={onClose}>{t("ui.cancel")}</Button><Button onClick={submit} loading={busy} disabled={!ip.trim() || (full && !replace)}>{full ? t("conn.replace") : t("ui.save")}</Button></>}>
      <div className="space-y-4">
        <Field label={t("dev.manual.ip")} hint={t("dev.manual.how")}>
          <Input value={ip} onChange={(e) => setIp(e.target.value)} ltr inputMode="decimal" placeholder="5.120.44.19" className="text-lg font-bold tracking-wide" />
        </Field>
        <Field label={t("conn.label")}>
          <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("conn.label.ph")} maxLength={40} />
        </Field>
        {full && (
          <div>
            <div className="text-sm font-semibold mb-2">{t("conn.replace.s")}</div>
            <div className="space-y-2">
              {me.devices.map((d) => (
                <button key={d.id} type="button" onClick={() => setReplace(d.id)}
                  className={cn("w-full flex items-center gap-3 p-3 rounded-2xl border text-start transition-colors", replace === d.id ? "border-primary bg-primary-soft" : "border-line hover:bg-surface-2")}>
                  <Laptop className="size-5 text-muted" />
                  <div className="flex-1 min-w-0">
                    <div className="font-semibold text-sm truncate">{d.label || t("dev.unnamed")}</div>
                    <div className="text-xs text-muted ltr text-start num">{d.ip}</div>
                  </div>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
