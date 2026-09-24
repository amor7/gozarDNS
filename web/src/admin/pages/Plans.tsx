import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, Package, Pencil, Plus, Search, Star, Trash2, Users as UsersIcon } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { bytes, GB, latinDigits, num, speed } from "@shared/format";
import { Badge, Button, Card, cn, Empty, Field, IconButton, Input, PageLoader, ServiceDot, Switch, Textarea } from "@shared/ui";
import { Modal, useConfirm, useToast } from "@shared/overlay";
import type { Catalog, Plan } from "../types";
import { PageHeader } from "../components";

const swatches = ["#7C5CFF", "#16C8A0", "#F59E0B", "#EF4444", "#3B82F6", "#EC4899", "#14B8A6", "#64748B"];

export function Plans() {
  const { t, lang } = useI18n();
  const plans = useQuery({ queryKey: ["plans"], queryFn: () => api<Plan[]>("/plans") });
  const [editing, setEditing] = useState<Partial<Plan> | null>(null);
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  if (!plans.data) return <PageLoader />;

  const remove = async (p: Plan) => {
    if (!(await confirm({ title: t("plans.delete.q", { name: p.name }), danger: true, confirm: t("ui.delete") }))) return;
    try {
      await api(`/plans/${p.id}`, { method: "DELETE" });
      qc.invalidateQueries({ queryKey: ["plans"] });
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <div>
      <PageHeader title={t("plans.title")} sub={t("plans.sub")}
        actions={<Button icon={<Plus className="size-4" />} onClick={() => setEditing({ max_devices: 2, duration_days: 30, quota_bytes: 50 * GB, groups: [], excluded: [], public: true, renewal: "expire", color: swatches[0] })}>{t("plans.new")}</Button>} />
      {plans.data.length === 0 ? (
        <Card><Empty icon={<Package className="size-6" />} title={t("plans.new")} /></Card>
      ) : (
        <div className="grid md:grid-cols-2 xl:grid-cols-3 gap-4">
          {plans.data.map((p) => (
            <Card key={p.id} className="relative overflow-hidden p-5 flex flex-col animate-in">
              <div className="absolute inset-x-0 top-0 h-1" style={{ background: p.color || "var(--primary)" }} />
              <div className="flex items-start gap-3">
                <div className="size-11 rounded-2xl grid place-items-center text-white shrink-0" style={{ background: p.color || "var(--primary)" }}>
                  <Package className="size-5" />
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-extrabold text-lg">{p.name}</span>
                    {p.is_default && <Badge tone="primary"><Star className="size-3" />{t("plans.default")}</Badge>}
                    {!p.public && <Badge>{t("plans.hidden")}</Badge>}
                  </div>
                  <div className="text-xs text-muted mt-0.5 line-clamp-2">{p.description}</div>
                </div>
              </div>
              <div className="text-2xl font-black mt-4 num">{num(p.price, lang)}</div>
              <div className="grid grid-cols-2 gap-2 mt-4 text-sm">
                <Spec k={t("plans.f.quota")} v={p.quota_bytes ? bytes(p.quota_bytes, lang) : t("ui.unlimited")} />
                <Spec k={t("plans.f.days")} v={p.duration_days ? num(p.duration_days, lang) : "∞"} />
                <Spec k={t("plans.f.speed")} v={speed(p.speed_kbps, lang)} />
                <Spec k={t("plans.f.devices")} v={num(p.max_devices, lang)} />
              </div>
              <div className="flex items-center gap-3 mt-4 text-xs text-muted">
                <span className="flex items-center gap-1"><UsersIcon className="size-3.5" />{t("plans.users", { n: num(p.users, lang) })}</span>
                <span>·</span>
                <span>{t("plans.domains", { n: num(p.domains ?? 0, lang) })}</span>
              </div>
              <div className="flex gap-2 mt-5 pt-4 border-t border-line">
                <Button variant="secondary" size="sm" className="flex-1" icon={<Pencil className="size-4" />} onClick={() => setEditing(p)}>{t("ui.edit")}</Button>
                <IconButton label="duplicate" onClick={() => setEditing({ ...p, id: undefined, name: p.name + " 2", is_default: false })}><Copy className="size-4" /></IconButton>
                <IconButton label={t("ui.delete")} className="hover:text-danger hover:bg-danger-soft" onClick={() => remove(p)}><Trash2 className="size-4" /></IconButton>
              </div>
            </Card>
          ))}
        </div>
      )}
      {editing && <PlanEditor plan={editing} onClose={() => setEditing(null)} />}
    </div>
  );
}

function Spec({ k, v }: { k: string; v: string }) {
  return (
    <div className="rounded-xl bg-surface-2 px-3 py-2">
      <div className="text-[11px] text-muted">{k}</div>
      <div className="font-bold num">{v}</div>
    </div>
  );
}

function PlanEditor({ plan, onClose }: { plan: Partial<Plan>; onClose: () => void }) {
  const { t, lang } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const cat = useQuery({ queryKey: ["catalog"], queryFn: () => api<Catalog>("/catalog") });
  const [f, setF] = useState({
    name: plan.name || "",
    description: plan.description || "",
    price: String(plan.price ?? 0),
    quota: plan.quota_bytes ? String(+(plan.quota_bytes / GB).toFixed(2)) : "0",
    days: String(plan.duration_days ?? 30),
    speed: plan.speed_kbps ? String(plan.speed_kbps / 1000) : "0",
    devices: String(plan.max_devices ?? 2),
    renewal: plan.renewal || "expire",
    public: plan.public ?? true,
    is_default: plan.is_default ?? false,
    color: plan.color || swatches[0],
    excluded: (plan.excluded || []).join("\n"),
  });
  const [groups, setGroups] = useState<Set<string>>(new Set(plan.groups || []));
  const [filter, setFilter] = useState("");
  const [busy, setBusy] = useState(false);

  const toggle = (k: string, on?: boolean) => {
    const g = new Set(groups);
    if (on ?? !g.has(k)) g.add(k);
    else g.delete(k);
    setGroups(g);
  };

  const byCategory = useMemo(() => {
    if (!cat.data) return [];
    const q = filter.trim().toLowerCase();
    return cat.data.categories
      .map((c) => ({
        c,
        services: cat.data!.services.filter((s) => s.category === c.id && (!q || s.name.en.toLowerCase().includes(q) || s.name.fa.includes(q) || s.groups.some((g) => g.domains.some((d) => d.includes(q))))),
      }))
      .filter((x) => x.services.length);
  }, [cat.data, filter]);

  const save = async () => {
    setBusy(true);
    const body = {
      name: f.name, description: f.description,
      price: Number(latinDigits(f.price).replace(/,/g, "")) || 0,
      quota_bytes: Math.round((Number(latinDigits(f.quota)) || 0) * GB),
      duration_days: Number(latinDigits(f.days)) || 0,
      speed_kbps: Math.round((Number(latinDigits(f.speed)) || 0) * 1000),
      max_devices: Number(latinDigits(f.devices)) || 1,
      renewal: f.renewal, public: f.public, is_default: f.is_default, color: f.color, sort: plan.sort ?? 0,
      groups: [...groups],
      excluded: f.excluded.split(/\s+/).map((s) => s.trim()).filter(Boolean),
    };
    try {
      await api(plan.id ? `/plans/${plan.id}` : "/plans", { method: plan.id ? "PUT" : "POST", body });
      toast.success(t("plans.saved"));
      qc.invalidateQueries({ queryKey: ["plans"] });
      onClose();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} size="xl" title={plan.id ? t("plans.edit.title") : t("plans.new.title")}
      footer={<><Button variant="secondary" onClick={onClose}>{t("ui.cancel")}</Button><Button onClick={save} loading={busy} disabled={!f.name.trim()}>{t("ui.save")}</Button></>}>
      <div className="grid lg:grid-cols-5 gap-6">
        <div className="lg:col-span-2 space-y-4">
          <Field label={t("plans.f.name")}><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} maxLength={40} /></Field>
          <Field label={t("plans.f.desc")}><Input value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} maxLength={300} /></Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label={t("plans.f.price")}><Input value={f.price} onChange={(e) => setF({ ...f, price: e.target.value })} ltr inputMode="numeric" /></Field>
            <Field label={t("plans.f.quota")} hint={t("user.quota.hint")}><Input value={f.quota} onChange={(e) => setF({ ...f, quota: e.target.value })} ltr inputMode="decimal" /></Field>
            <Field label={t("plans.f.days")} hint={t("plans.f.days.hint")}><Input value={f.days} onChange={(e) => setF({ ...f, days: e.target.value })} ltr inputMode="numeric" /></Field>
            <Field label={t("plans.f.speed")} hint={t("user.speed.hint")}><Input value={f.speed} onChange={(e) => setF({ ...f, speed: e.target.value })} ltr inputMode="decimal" /></Field>
            <Field label={t("plans.f.devices")}><Input value={f.devices} onChange={(e) => setF({ ...f, devices: e.target.value })} ltr inputMode="numeric" /></Field>
            <Field label={t("plans.f.color")}>
              <div className="flex flex-wrap gap-1.5 pt-1">
                {swatches.map((c) => (
                  <button key={c} type="button" onClick={() => setF({ ...f, color: c })} className="size-7 rounded-lg grid place-items-center" style={{ background: c }}>
                    {f.color === c && <Check className="size-4 text-white" strokeWidth={3} />}
                  </button>
                ))}
              </div>
            </Field>
          </div>
          <Field label={t("plans.f.renewal")}>
            <div className="grid grid-cols-2 gap-2">
              {(["expire", "reset"] as const).map((r) => (
                <button key={r} type="button" onClick={() => setF({ ...f, renewal: r })}
                  className={cn("h-11 rounded-xl border text-sm font-semibold", f.renewal === r ? "border-primary bg-primary-soft text-primary" : "border-line hover:bg-surface-2")}>
                  {t(`plans.f.renewal.${r}`)}
                </button>
              ))}
            </div>
          </Field>
          <div className="space-y-3 pt-1">
            <Switch checked={f.public} onChange={(v) => setF({ ...f, public: v })} label={t("plans.f.public")} />
            <Switch checked={f.is_default} onChange={(v) => setF({ ...f, is_default: v })} label={t("plans.f.default")} />
          </div>
          <Field label={t("plans.f.excluded")} hint={t("plans.f.excluded.hint")}>
            <Textarea value={f.excluded} onChange={(e) => setF({ ...f, excluded: e.target.value })} className="ltr font-mono text-[13px]" rows={3} />
          </Field>
        </div>

        <div className="lg:col-span-3">
          <div className="flex items-center justify-between gap-2 mb-3">
            <div>
              <div className="font-bold text-sm">{t("plans.f.services")}</div>
              <div className="text-xs text-muted">{t("plans.f.services.sub", { n: num(groups.size, lang) })}</div>
            </div>
            <div className="flex gap-1">
              <Button size="sm" variant="ghost" onClick={() => cat.data && setGroups(new Set([...cat.data.services.flatMap((s) => s.groups.map((g) => `${s.id}/${g.id}`)), cat.data.custom_key]))}>{t("plans.f.all")}</Button>
              <Button size="sm" variant="ghost" onClick={() => setGroups(new Set())}>{t("plans.f.clear")}</Button>
            </div>
          </div>
          <div className="relative mb-3">
            <Search className="size-4 absolute top-1/2 -translate-y-1/2 start-3.5 text-faint" />
            <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder={t("ui.search")} className="ps-10 h-10" />
          </div>
          <div className="space-y-4 lg:max-h-[56vh] overflow-y-auto pe-1">
            {cat.data && (
              <label className="flex items-center gap-3 p-3 rounded-2xl border border-dashed border-line-strong cursor-pointer hover:bg-surface-2">
                <input type="checkbox" className="size-4 accent-[var(--primary)]" checked={groups.has(cat.data.custom_key)} onChange={() => toggle(cat.data!.custom_key)} />
                <span className="text-sm font-semibold">{t("plans.f.custom")}</span>
              </label>
            )}
            {byCategory.map(({ c, services }) => {
              const keys = services.flatMap((s) => s.groups.map((g) => `${s.id}/${g.id}`));
              const all = keys.every((k) => groups.has(k));
              return (
                <div key={c.id}>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs font-bold text-muted">{c.name[lang]}</span>
                    <button className="text-xs text-primary font-semibold" onClick={() => { const g = new Set(groups); keys.forEach((k) => (all ? g.delete(k) : g.add(k))); setGroups(g); }}>
                      {all ? t("plans.f.clear") : t("plans.f.all")}
                    </button>
                  </div>
                  <div className="grid sm:grid-cols-2 gap-2">
                    {services.map((s) => (
                      <div key={s.id} className={cn("rounded-2xl border p-2.5 transition-colors", s.groups.some((g) => groups.has(`${s.id}/${g.id}`)) ? "border-primary/40 bg-primary-soft" : "border-line")}>
                        <div className="flex items-center gap-2.5">
                          <ServiceDot name={s.name.en} color={s.color} size={26} />
                          <span className="text-sm font-semibold flex-1 truncate">{s.name[lang]}</span>
                          {s.groups.length === 1 && (
                            <input type="checkbox" className="size-4 accent-[var(--primary)]" checked={groups.has(`${s.id}/${s.groups[0].id}`)} onChange={() => toggle(`${s.id}/${s.groups[0].id}`)} />
                          )}
                        </div>
                        {s.groups.length > 1 && (
                          <div className="mt-2 space-y-1 ps-9">
                            {s.groups.map((g) => (
                              <label key={g.id} className="flex items-center gap-2 text-xs cursor-pointer">
                                <input type="checkbox" className="size-3.5 accent-[var(--primary)]" checked={groups.has(`${s.id}/${g.id}`)} onChange={() => toggle(`${s.id}/${g.id}`)} />
                                <span className="flex-1">{g.name[lang]}</span>
                                {g.heavy && <Badge tone="warning" className="h-5 text-[10px]">{t("plans.f.heavy")}</Badge>}
                              </label>
                            ))}
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </Modal>
  );
}
