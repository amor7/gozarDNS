import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarPlus, KeyRound, Laptop, MoreHorizontal, Plus, Power, RotateCcw, Trash2, X } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { bytes, date, daysLeft, GB, latinDigits, money, num, relative, shortDate, speed } from "@shared/format";
import { Avatar, Badge, Button, Card, CopyButton, Field, IconButton, Input, PageLoader, Progress, Segmented, Select, cn } from "@shared/ui";
import { BarChart } from "@shared/charts";
import { Drawer, Modal, useConfirm, useToast } from "@shared/overlay";
import type { Device, Payment, Plan, User } from "../types";
import { KV, StateBadge } from "../components";

type Detail = { user: User; devices: Device[]; usage: { day: string; bytes: number }[]; payments: Payment[] };

export function UserDrawer({ id, onClose }: { id: number; onClose: () => void }) {
  const { t, lang } = useI18n();
  const q = useQuery({ queryKey: ["user", id], queryFn: () => api<Detail>(`/users/${id}`) });
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [tab, setTab] = useState<"overview" | "edit" | "devices" | "payments">("overview");
  const [planOpen, setPlanOpen] = useState(false);
  const [menu, setMenu] = useState(false);
  const [newPw, setNewPw] = useState<string | null>(null);

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["user", id] });
    qc.invalidateQueries({ queryKey: ["users"] });
  };
  const run = async (fn: () => Promise<unknown>, ok?: string) => {
    try {
      await fn();
      if (ok) toast.success(ok);
      refresh();
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <Drawer open onClose={onClose}>
      {!q.data ? (
        <PageLoader />
      ) : (
        (() => {
          const { user: u, devices, usage, payments } = q.data;
          const unlimited = u.quota_bytes === 0;
          const days = daysLeft(u.expires_at);
          return (
            <div>
              <div className="sticky top-0 z-10 glass border-x-0 border-t-0 px-5 py-4 flex items-center gap-3">
                <Avatar name={u.username} size={44} />
                <div className="flex-1 min-w-0">
                  <div className="font-extrabold text-lg truncate">{u.display_name || u.username}</div>
                  <div className="text-xs text-muted ltr text-start">@{u.username}{u.phone && ` · ${u.phone}`}</div>
                </div>
                <StateBadge state={u.state} />
                <div className="relative">
                  <IconButton label={t("ui.more")} onClick={() => setMenu(!menu)}><MoreHorizontal className="size-5" /></IconButton>
                  {menu && (
                    <>
                      <div className="fixed inset-0" onClick={() => setMenu(false)} />
                      <div className="absolute end-0 top-11 w-56 card shadow-pop p-1.5 animate-in z-10">
                        <MenuItem icon={<RotateCcw className="size-4" />} onClick={async () => {
                          setMenu(false);
                          if (await confirm({ title: t("user.reset.q") })) run(() => api(`/users/${id}/reset-usage`, { method: "POST" }), t("user.reset.done"));
                        }}>{t("user.reset")}</MenuItem>
                        <MenuItem icon={<KeyRound className="size-4" />} onClick={async () => {
                          setMenu(false);
                          if (await confirm({ title: t("user.password.q"), body: t("user.password.s") })) {
                            try {
                              const r = await api<{ password: string }>(`/users/${id}/password`, { body: {} });
                              setNewPw(r.password);
                            } catch (e) { toast.error(e); }
                          }
                        }}>{t("user.password")}</MenuItem>
                        <MenuItem icon={<Power className="size-4" />} onClick={() => { setMenu(false); run(() => api(`/users/${id}`, { method: "PATCH", body: { enabled: !u.enabled } })); }}>
                          {u.enabled ? t("user.disable") : t("user.enable")}
                        </MenuItem>
                        <div className="h-px bg-line my-1" />
                        <MenuItem danger icon={<Trash2 className="size-4" />} onClick={async () => {
                          setMenu(false);
                          if (await confirm({ title: t("user.delete.q", { name: u.username }), body: t("user.delete.s"), confirm: t("ui.delete"), danger: true })) {
                            try { await api(`/users/${id}`, { method: "DELETE" }); toast.success(t("user.deleted")); qc.invalidateQueries({ queryKey: ["users"] }); onClose(); } catch (e) { toast.error(e); }
                          }
                        }}>{t("user.delete")}</MenuItem>
                      </div>
                    </>
                  )}
                </div>
                <IconButton label={t("ui.close")} onClick={onClose}><X className="size-5" /></IconButton>
              </div>

              <div className="p-5 space-y-4">
                {/* Headline: usage and time */}
                <Card className="p-5">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="text-xs text-muted">{t("user.plan")}</div>
                      <div className="font-extrabold text-lg">{u.plan_name || "—"}</div>
                    </div>
                    <Button size="sm" variant="soft" icon={<CalendarPlus className="size-4" />} onClick={() => setPlanOpen(true)}>{t("user.plan.change")}</Button>
                  </div>
                  <div className="mt-4">
                    <div className="flex items-baseline justify-between mb-2">
                      <span className="text-2xl font-black num">{bytes(u.used_bytes, lang)}</span>
                      <span className="text-sm text-muted num">{unlimited ? t("ui.unlimited") : `/ ${bytes(u.quota_bytes, lang)}`}</span>
                    </div>
                    <Progress value={u.used_bytes} max={u.quota_bytes} className="h-2.5" />
                  </div>
                  <div className="grid grid-cols-3 gap-2 mt-4 text-center">
                    <Box label={t("user.expires")} value={days === null ? "∞" : t("users.days", { n: num(days, lang) })} sub={u.expires_at ? date(u.expires_at, lang) : undefined} />
                    <Box label={t("user.speed")} value={speed(u.speed_kbps, lang)} />
                    <Box label={t("user.devices")} value={`${num(u.devices, lang)}/${num(u.max_devices, lang)}`} />
                  </div>
                </Card>

                <Segmented value={tab} onChange={setTab} className="w-full [&>button]:flex-1" options={[
                  { value: "overview", label: t("user.overview") },
                  { value: "edit", label: t("user.edit") },
                  { value: "devices", label: `${t("user.devices")} · ${num(devices.length, lang)}` },
                  { value: "payments", label: t("user.payments") },
                ]} />

                {tab === "overview" && (
                  <>
                    <Card className="p-5">
                      <div className="text-sm font-bold mb-3">{t("user.usage")}</div>
                      <BarChart data={usage.map((d) => ({ key: d.day, value: d.bytes }))} height={120} format={(v) => bytes(v, lang)} label={(k) => shortDate(k, lang)} />
                    </Card>
                    <Card className="px-5 py-2">
                      <KV k={t("user.created")} v={date(u.created_at, lang)} />
                      <KV k={t("user.lastseen")} v={u.last_seen_at ? relative(u.last_seen_at, lang) : t("ui.never")} />
                      {u.note && <KV k={t("user.note")} v={<span className="font-normal">{u.note}</span>} />}
                    </Card>
                  </>
                )}
                {tab === "edit" && <EditForm u={u} onSaved={refresh} />}
                {tab === "devices" && <DevicesTab id={id} devices={devices} onChange={refresh} />}
                {tab === "payments" && (
                  <Card className="divide-y divide-line">
                    {payments.length === 0 && <div className="text-sm text-muted text-center py-8">{t("user.payments.none")}</div>}
                    {payments.map((p) => (
                      <div key={p.id} className="flex items-center gap-3 px-4 py-3">
                        <div className="flex-1">
                          <div className="text-sm font-semibold">{p.plan_name || "—"}</div>
                          <div className="text-xs text-muted">{date(p.created_at, lang, true)}</div>
                        </div>
                        <span className="text-sm font-bold num">{money(p.amount, lang, "")}</span>
                        <Badge tone={p.status === "approved" ? "success" : p.status === "rejected" ? "danger" : "warning"}>{t("pay.status." + p.status)}</Badge>
                      </div>
                    ))}
                  </Card>
                )}
              </div>

              {planOpen && <PlanModal u={u} onClose={() => setPlanOpen(false)} onDone={refresh} />}
              <Modal open={!!newPw} onClose={() => setNewPw(null)} title={t("user.password.done")} size="sm" footer={<CopyButton text={newPw || ""} size="md" />}>
                <div className="rounded-2xl bg-surface-2 border border-line p-4 text-center font-mono text-xl font-bold ltr tracking-wider">{newPw}</div>
              </Modal>
            </div>
          );
        })()
      )}
    </Drawer>
  );
}

function MenuItem({ icon, children, onClick, danger }: { icon: React.ReactNode; children: React.ReactNode; onClick: () => void; danger?: boolean }) {
  return (
    <button onClick={onClick} className={cn("w-full h-10 px-3 rounded-xl flex items-center gap-2.5 text-sm font-semibold text-start hover:bg-surface-2", danger && "text-danger hover:bg-danger-soft")}>
      {icon}
      {children}
    </button>
  );
}

function Box({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-xl bg-surface-2 border border-line p-2.5">
      <div className="text-[11px] text-muted">{label}</div>
      <div className="font-bold text-sm mt-0.5 num">{value}</div>
      {sub && <div className="text-[10px] text-faint mt-0.5">{sub}</div>}
    </div>
  );
}

function EditForm({ u, onSaved }: { u: User; onSaved: () => void }) {
  const { t, lang } = useI18n();
  const toast = useToast();
  const initialDays = u.expires_at ? String(daysLeft(u.expires_at)) : "";
  const [f, setF] = useState({
    display_name: u.display_name, phone: u.phone, note: u.note,
    quota: u.quota_bytes ? String(+(u.quota_bytes / GB).toFixed(2)) : "0",
    days: initialDays,
    speed: u.speed_kbps ? String(u.speed_kbps / 1000) : "0",
    max_devices: String(u.max_devices),
  });
  useEffect(() => setF((x) => ({ ...x, days: initialDays })), [initialDays]);
  const [busy, setBusy] = useState(false);
  const daysN = f.days.trim() === "" ? null : Number(latinDigits(f.days));
  const preview = daysN === null ? null : Math.floor(Date.now() / 1000 + daysN * 86400);

  const save = async () => {
    setBusy(true);
    const body: Record<string, unknown> = {
      display_name: f.display_name, phone: f.phone, note: f.note,
      quota_bytes: Math.round(Number(latinDigits(f.quota) || 0) * GB),
      speed_kbps: Math.round(Number(latinDigits(f.speed) || 0) * 1000),
      max_devices: Number(latinDigits(f.max_devices) || 1),
    };
    if (f.days !== initialDays) body.expires_at = preview ?? 0;
    try {
      await api(`/users/${u.id}`, { method: "PATCH", body });
      toast.success(t("ui.saved"));
      onSaved();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="p-5">
      <div className="grid sm:grid-cols-2 gap-4">
        <Field label={t("user.name")}><Input value={f.display_name} onChange={(e) => setF({ ...f, display_name: e.target.value })} /></Field>
        <Field label={t("user.phone")}><Input value={f.phone} onChange={(e) => setF({ ...f, phone: e.target.value })} ltr /></Field>
        <Field label={t("user.quota")} hint={t("user.quota.hint")}><Input value={f.quota} onChange={(e) => setF({ ...f, quota: e.target.value })} ltr inputMode="decimal" /></Field>
        <Field label={`${t("user.expires")} (${t("ui.days")})`} hint={preview ? date(preview, lang) : t("user.expires.hint")}>
          <Input value={f.days} onChange={(e) => setF({ ...f, days: e.target.value })} ltr inputMode="numeric" placeholder="∞" />
        </Field>
        <Field label={t("user.speed")} hint={t("user.speed.hint")}><Input value={f.speed} onChange={(e) => setF({ ...f, speed: e.target.value })} ltr inputMode="decimal" /></Field>
        <Field label={t("user.maxdev")}><Input value={f.max_devices} onChange={(e) => setF({ ...f, max_devices: e.target.value })} ltr inputMode="numeric" /></Field>
        <Field label={t("user.note")} hint={t("user.note.hint")} className="sm:col-span-2"><Input value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} /></Field>
      </div>
      <Button className="mt-5" onClick={save} loading={busy}>{t("ui.save")}</Button>
    </Card>
  );
}

function DevicesTab({ id, devices, onChange }: { id: number; devices: Device[]; onChange: () => void }) {
  const { t, lang } = useI18n();
  const toast = useToast();
  const confirm = useConfirm();
  const [ip, setIp] = useState("");
  const [label, setLabel] = useState("");
  const add = async () => {
    try {
      await api(`/users/${id}/devices`, { body: { ip: latinDigits(ip.trim()), label } });
      setIp("");
      setLabel("");
      onChange();
    } catch (e) {
      toast.error(e);
    }
  };
  return (
    <Card>
      <div className="divide-y divide-line">
        {devices.length === 0 && <div className="text-sm text-muted text-center py-8">{t("user.devices.none")}</div>}
        {devices.map((d) => (
          <div key={d.id} className="flex items-center gap-3 px-4 py-3">
            <span className="size-9 rounded-xl bg-surface-2 grid place-items-center"><Laptop className="size-4 text-muted" /></span>
            <div className="flex-1 min-w-0">
              <div className="text-sm font-semibold ltr text-start num">{d.ip}</div>
              <div className="text-xs text-muted">{d.label || "—"} · {d.last_seen_at ? relative(d.last_seen_at, lang) : t("ui.never")}</div>
            </div>
            <IconButton label={t("ui.delete")} className="hover:text-danger hover:bg-danger-soft" onClick={async () => {
              if (await confirm({ title: t("user.devices.remove.q", { ip: d.ip }), danger: true, confirm: t("ui.delete") })) {
                try { await api(`/users/${id}/devices/${d.id}`, { method: "DELETE" }); onChange(); } catch (e) { toast.error(e); }
              }
            }}><Trash2 className="size-4" /></IconButton>
          </div>
        ))}
      </div>
      <div className="p-4 border-t border-line flex flex-col sm:flex-row gap-2">
        <Input value={ip} onChange={(e) => setIp(e.target.value)} placeholder={t("user.devices.ip")} ltr className="sm:flex-1" />
        <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("user.devices.label")} className="sm:flex-1" />
        <Button onClick={add} disabled={!ip} icon={<Plus className="size-4" />}>{t("user.devices.add")}</Button>
      </div>
    </Card>
  );
}

function PlanModal({ u, onClose, onDone }: { u: User; onClose: () => void; onDone: () => void }) {
  const { t, lang } = useI18n();
  const toast = useToast();
  const plans = useQuery({ queryKey: ["plans"], queryFn: () => api<Plan[]>("/plans") });
  const [planId, setPlanId] = useState<number>(u.plan_id || 0);
  const [mode, setMode] = useState<"extend" | "apply">(u.plan_id && u.state !== "expired" ? "extend" : "apply");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!planId && plans.data?.length) setPlanId(plans.data[0].id);
  }, [plans.data, planId]);
  const p = plans.data?.find((x) => x.id === planId);
  const submit = async () => {
    setBusy(true);
    try {
      await api(`/users/${u.id}/plan`, { body: { plan_id: planId, mode } });
      toast.success(t("user.plan.done"));
      onDone();
      onClose();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={t("user.plan.change")}
      footer={<><Button variant="secondary" onClick={onClose}>{t("ui.cancel")}</Button><Button onClick={submit} loading={busy} disabled={!planId}>{t("ui.confirm")}</Button></>}>
      <Field label={t("user.plan")}>
        <Select value={planId} onChange={(e) => setPlanId(Number(e.target.value))}>
          {plans.data?.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
        </Select>
      </Field>
      {p && (
        <div className="text-xs text-muted mt-2 num">
          {p.quota_bytes ? bytes(p.quota_bytes, lang) : t("ui.unlimited")} · {p.duration_days ? `${num(p.duration_days, lang)} ${t("ui.days")}` : "∞"} · {speed(p.speed_kbps, lang)}
        </div>
      )}
      <div className="grid gap-2 mt-4">
        {(["extend", "apply"] as const).map((m) => (
          <button key={m} type="button" onClick={() => setMode(m)} className={cn("text-start p-3.5 rounded-2xl border transition-colors", mode === m ? "border-primary bg-primary-soft" : "border-line hover:bg-surface-2")}>
            <div className="font-bold text-sm">{t(`user.plan.${m}`)}</div>
            <div className="text-xs text-muted mt-0.5">{t(`user.plan.${m}.s`)}</div>
          </button>
        ))}
      </div>
    </Modal>
  );
}
