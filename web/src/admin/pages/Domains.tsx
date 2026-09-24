import { useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Globe, Plus, Search, ShieldAlert, Trash2 } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { num, relative } from "@shared/format";
import { Badge, Button, Card, CardHeader, cn, Empty, IconButton, Input, PageLoader, Segmented, ServiceDot, Switch } from "@shared/ui";
import { useToast } from "@shared/overlay";
import type { Catalog, Text } from "../types";
import { PageHeader } from "../components";

type Lookup = { name: string; owner: { service: Text; group: Text; domain: string } | null; plans: { plan_id: number; plan: string; action: string; matched: string }[] };

export function Domains() {
  const { t, lang } = useI18n();
  const cat = useQuery({ queryKey: ["catalog"], queryFn: () => api<Catalog>("/catalog") });
  if (!cat.data) return <PageLoader />;
  const c = cat.data;
  const total = c.services.reduce((n, s) => n + s.groups.reduce((m, g) => m + g.domains.length, 0), 0);
  return (
    <div className="space-y-5">
      <PageHeader title={t("domains.title")} sub={t("domains.sub")} />
      <LookupCard />
      <div className="grid xl:grid-cols-2 gap-5">
        <CustomDomains c={c} />
        <Rules c={c} />
      </div>
      <CatalogView c={c} total={total} lang={lang} />
    </div>
  );
}

function LookupCard() {
  const { t, lang } = useI18n();
  const toast = useToast();
  const [name, setName] = useState("");
  const [res, setRes] = useState<Lookup | null>(null);
  const [busy, setBusy] = useState(false);
  const run = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      setRes(await api<Lookup>(`/lookup?name=${encodeURIComponent(name.trim())}`));
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };
  const tone = (a: string) => (a === "route" ? "success" : a === "pin" ? "info" : "neutral");
  return (
    <Card className="p-5">
      <form onSubmit={run} className="flex flex-col sm:flex-row gap-2">
        <div className="relative flex-1">
          <Search className="size-4 absolute top-1/2 -translate-y-1/2 start-3.5 text-faint" />
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("domains.lookup.ph")} ltr className="ps-10" />
        </div>
        <Button type="submit" loading={busy} disabled={!name.trim()}>{t("domains.lookup.run")}</Button>
      </form>
      {res && (
        <div className="mt-4 animate-in">
          <div className="text-sm text-muted mb-3">
            {res.owner ? t("domains.lookup.owner", { service: res.owner.service.en, group: res.owner.group[lang] }) : t("domains.lookup.noowner")}
          </div>
          <div className="flex flex-wrap gap-2">
            {res.plans.map((p) => (
              <div key={p.plan_id} className="flex items-center gap-2 rounded-xl bg-surface-2 border border-line ps-3 pe-1.5 py-1.5">
                <span className="text-sm font-semibold">{p.plan}</span>
                <Badge tone={tone(p.action)}>{t("domains.action." + p.action)}</Badge>
              </div>
            ))}
          </div>
        </div>
      )}
    </Card>
  );
}

function CustomDomains({ c }: { c: Catalog }) {
  const { t, lang: l } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const [domain, setDomain] = useState("");
  const [note, setNote] = useState("");
  const [action, setAction] = useState<"route" | "direct">("route");
  const add = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await api("/domains", { body: { domain: domain.trim(), action, note } });
      setDomain("");
      setNote("");
      qc.invalidateQueries({ queryKey: ["catalog"] });
    } catch (e) {
      toast.error(e);
    }
  };
  const del = async (d: string) => {
    try {
      await api(`/domains/${encodeURIComponent(d)}`, { method: "DELETE" });
      qc.invalidateQueries({ queryKey: ["catalog"] });
    } catch (e) {
      toast.error(e);
    }
  };
  return (
    <Card>
      <CardHeader title={t("domains.custom")} sub={t("domains.custom.sub")} icon={<Plus className="size-[18px]" />} />
      <form onSubmit={add} className="px-5 pb-4 space-y-2">
        <div className="flex gap-2">
          <Input value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="example.com" ltr className="flex-1" />
          <Segmented value={action} onChange={setAction} options={[{ value: "route", label: t("domains.action.route") }, { value: "direct", label: t("domains.action.direct") }]} className="shrink-0" />
        </div>
        <div className="flex gap-2">
          <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("domains.custom.note")} className="flex-1" />
          <Button type="submit" disabled={!domain.trim()}>{t("ui.add")}</Button>
        </div>
      </form>
      <div className="border-t border-line divide-y divide-line">
        {c.custom.length === 0 && <div className="text-sm text-muted text-center py-6">{t("domains.custom.none")}</div>}
        {c.custom.map((d) => (
          <div key={d.domain} className="flex items-center gap-3 px-5 py-2.5">
            <Globe className="size-4 text-faint" />
            <div className="flex-1 min-w-0">
              <div className="text-sm font-semibold ltr text-start truncate">{d.domain}</div>
              <div className="text-xs text-muted truncate">{d.note || relative(d.added_at, l)}</div>
            </div>
            <Badge tone={d.action === "route" ? "success" : "neutral"}>{t("domains.action." + d.action)}</Badge>
            <IconButton label={t("ui.delete")} className="hover:text-danger hover:bg-danger-soft" onClick={() => del(d.domain)}><Trash2 className="size-4" /></IconButton>
          </div>
        ))}
      </div>
    </Card>
  );
}

function Rules({ c }: { c: Catalog }) {
  const { t, lang: l } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const set = async (id: string, enabled: boolean) => {
    try {
      await api(`/rules/${id}`, { method: "PUT", body: { enabled } });
      qc.invalidateQueries({ queryKey: ["catalog"] });
    } catch (e) {
      toast.error(e);
    }
  };
  return (
    <Card>
      <CardHeader title={t("domains.rules")} sub={t("domains.rules.sub")} icon={<ShieldAlert className="size-[18px]" />} />
      <div className="border-t border-line divide-y divide-line">
        {c.rules.map((r) => (
          <div key={r.id} className="flex items-start gap-3 px-5 py-3.5">
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <span className="text-sm font-bold">{r.title[l]}</span>
                <Badge tone={r.action === "pin" ? "info" : "neutral"}>{t("domains.action." + r.action)}</Badge>
              </div>
              <div className="text-xs text-muted mt-1 leading-relaxed">{r.why[l]}</div>
              <div className="text-[11px] text-faint mt-1 ltr text-start truncate">{r.domains.slice(0, 3).join(", ")}{r.domains.length > 3 && " …"}</div>
            </div>
            <Switch checked={r.enabled} onChange={(v) => set(r.id, v)} />
          </div>
        ))}
      </div>
    </Card>
  );
}

function CatalogView({ c, total, lang: l }: { c: Catalog; total: number; lang: "fa" | "en" }) {
  const { t } = useI18n();
  const [open, setOpen] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const services = c.services.filter((s) => !q || s.name.en.toLowerCase().includes(q.toLowerCase()) || s.name.fa.includes(q) || s.groups.some((g) => g.domains.some((d) => d.includes(q.toLowerCase()))));
  return (
    <Card>
      <CardHeader title={t("domains.catalog")} sub={t("domains.catalog.sub", { s: num(c.services.length, l), d: num(total, l) })}
        action={<div className="relative w-48 sm:w-64"><Search className="size-4 absolute top-1/2 -translate-y-1/2 start-3 text-faint" /><Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("ui.search")} className="h-9 ps-9" /></div>} />
      {services.length === 0 ? (
        <Empty icon={<Search className="size-6" />} title={t("nav.search.empty")} />
      ) : (
        <div className="border-t border-line divide-y divide-line">
          {services.map((s) => {
            const count = s.groups.reduce((n, g) => n + g.domains.length, 0);
            const isOpen = open === s.id || !!q;
            return (
              <div key={s.id}>
                <button onClick={() => setOpen(open === s.id ? null : s.id)} className="w-full flex items-center gap-3 px-5 py-3 hover:bg-surface-2 text-start">
                  <ServiceDot name={s.name.en} color={s.color} />
                  <span className="flex-1 font-semibold text-sm">{s.name[l]}</span>
                  <span className="text-xs text-muted">{t("domains.count", { n: num(count, l) })}</span>
                  <ChevronDown className={cn("size-4 text-faint transition-transform", isOpen && "rotate-180")} />
                </button>
                {isOpen && (
                  <div className="px-5 pb-4 space-y-3 animate-in">
                    {s.groups.map((g) => (
                      <div key={g.id}>
                        <div className="text-xs font-bold text-muted mb-1.5 flex items-center gap-2">{g.name[l]}{g.heavy && <Badge tone="warning" className="h-5 text-[10px]">{t("plans.f.heavy")}</Badge>}</div>
                        <div className="flex flex-wrap gap-1.5">
                          {g.domains.map((d) => (
                            <code key={d} className={cn("text-xs px-2 py-1 rounded-lg border ltr", q && d.includes(q.toLowerCase()) ? "bg-primary-soft border-primary/30 text-primary" : "bg-surface-2 border-line")}>{d}</code>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
