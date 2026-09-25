import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { renderSVG } from "uqr";
import { CreditCard, DatabaseBackup, Download, Headset, Info, KeyRound, Plus, ShieldCheck, SlidersHorizontal, Trash2, Upload, Users as UsersIcon } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { date, latinDigits, relative } from "@shared/format";
import { Avatar, Badge, Button, Card, CardHeader, cn, Field, IconButton, Input, PageLoader, PasswordInput, Select, Switch, Textarea } from "@shared/ui";
import { Modal, useConfirm, useToast } from "@shared/overlay";
import type { Admin, Plan } from "../types";
import { gatewayNames } from "@shared/gateways";
import { KV, PageHeader } from "../components";
import { useMe } from "../App";

type SettingsResp = {
  settings: Record<string, string>;
  system: { version: string; public_ip: string; admin_url: string; admin_domain: string; cert_until: number; ipv6_egress: boolean; control_port: number; fingerprint: string };
};

/** One draft for every editable setting on the page. Only changed keys live
 *  here, so a refetch of the settings can never overwrite what is being
 *  typed; the save bar sends exactly these. */
type Form = {
  v: (k: string) => string;
  set: (k: string, val: string) => void;
  secret: (k: string) => string;
  setSecret: (k: string, val: string) => void;
};

export function Settings() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["settings"], queryFn: () => api<SettingsResp>("/settings") });
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [secrets, setSecrets] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const dirty = Object.keys(draft).length > 0 || Object.values(secrets).some((x) => x.trim());

  // Leaving with unsaved changes asks first.
  useEffect(() => {
    if (!dirty) return;
    const on = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", on);
    return () => window.removeEventListener("beforeunload", on);
  }, [dirty]);

  if (!q.data) return <PageLoader />;
  const s = q.data.settings;
  const form: Form = {
    v: (k) => (k in draft ? draft[k] : s[k] || ""),
    set: (k, val) => setDraft((d) => ({ ...d, [k]: val })),
    secret: (k) => secrets[k] || "",
    setSecret: (k, val) => setSecrets((x) => ({ ...x, [k]: val })),
  };

  const save = async () => {
    setBusy(true);
    try {
      const body: Record<string, string> = { ...draft };
      if ("dns_address" in body) body.dns_address = latinDigits(body.dns_address);
      for (const [k, val] of Object.entries(secrets)) if (val.trim()) body[k] = val.trim();
      await api("/settings", { method: "PATCH", body });
      setDraft({});
      setSecrets({});
      toast.success(t("ui.saved"));
      qc.invalidateQueries({ queryKey: ["settings"] });
      qc.invalidateQueries({ queryKey: ["me"] });
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className={cn("space-y-5", dirty && "pb-24")}>
      <PageHeader title={t("set.title")} />
      <div className="grid xl:grid-cols-2 gap-5 items-start">
        <div className="space-y-5">
          <General form={form} />
          <Support form={form} />
          <OnlinePay s={s} form={form} />
          <Backup />
        </div>
        <div className="space-y-5">
          <Security />
          <Admins />
          <System sys={q.data.system} />
        </div>
      </div>
      {dirty && (
        <div className="fixed z-40 bottom-4 inset-x-4 lg:start-[288px] lg:end-8 animate-up">
          <div className="max-w-3xl mx-auto glass rounded-2xl shadow-pop px-4 py-3 flex items-center gap-3">
            <span className="size-2.5 rounded-full bg-warning shrink-0" />
            <span className="flex-1 text-sm font-semibold">{t("set.unsaved")}</span>
            <Button variant="ghost" size="sm" onClick={() => { setDraft({}); setSecrets({}); }}>{t("ui.cancel")}</Button>
            <Button size="sm" onClick={save} loading={busy}>{t("set.save")}</Button>
          </div>
        </div>
      )}
    </div>
  );
}

function General({ form }: { form: Form }) {
  const { t } = useI18n();
  const plans = useQuery({ queryKey: ["plans"], queryFn: () => api<Plan[]>("/plans") });
  const bind = (k: string) => ({ value: form.v(k), onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => form.set(k, e.target.value) });
  return (
    <Card>
      <CardHeader title={t("set.general")} icon={<SlidersHorizontal className="size-[18px]" />} />
      <div className="px-5 pb-5 space-y-4">
        <div className="grid sm:grid-cols-3 gap-3">
          <Field label={t("set.brand")} hint={t("set.brand.hint")} className="sm:col-span-2"><Input {...bind("brand")} /></Field>
          <Field label={t("set.currency")}><Input {...bind("currency")} /></Field>
        </div>
        <Field label={t("set.notice")} hint={t("set.notice.hint")}><Textarea {...bind("notice")} rows={2} /></Field>
        <Field label={t("set.payment")} hint={t("set.payment.hint")}><Textarea {...bind("payment_info")} rows={3} /></Field>
        <div className="rounded-2xl border border-line p-4 space-y-3">
          <div className="font-bold text-sm">{t("set.signup")}</div>
          <Switch checked={form.v("signup_open") === "1"} onChange={(x) => form.set("signup_open", x ? "1" : "0")} label={t("set.signup.open")} />
          <Field label={t("set.signup.plan")}>
            <Select value={form.v("signup_plan") || "0"} onChange={(e) => form.set("signup_plan", e.target.value)}>
              <option value="0">{t("set.signup.plan.none")}</option>
              {plans.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </Select>
          </Field>
        </div>
        <Field label={t("set.dns")} hint={t("set.dns.hint")}><Input {...bind("dns_address")} ltr placeholder="—" /></Field>
      </div>
    </Card>
  );
}

const contactFields: { key: string; ltr?: boolean; ph: string; mode?: "tel" | "email" }[] = [
  { key: "support_telegram", ltr: true, ph: "@gozar_support" },
  { key: "support_whatsapp", ltr: true, ph: "0912 345 6789", mode: "tel" },
  { key: "support_bale", ltr: true, ph: "@gozar_support" },
  { key: "support_phone", ltr: true, ph: "021 1234 5678", mode: "tel" },
  { key: "support_email", ltr: true, ph: "support@example.com", mode: "email" },
  { key: "support_hours", ph: "" },
];

function Support({ form }: { form: Form }) {
  const { t } = useI18n();
  return (
    <Card>
      <CardHeader title={t("set.contact")} sub={t("set.contact.sub")} icon={<Headset className="size-[18px]" />} />
      <div className="px-5 pb-5 space-y-4">
        <div className="grid sm:grid-cols-2 gap-3">
          {contactFields.map((c) => (
            <Field key={c.key} label={t("set." + c.key)}>
              <Input value={form.v(c.key)} onChange={(e) => form.set(c.key, e.target.value)} ltr={c.ltr} inputMode={c.mode} placeholder={c.ph || t("set.support_hours.ph")} />
            </Field>
          ))}
        </div>
        <Field label={t("set.support")} hint={t("set.support.hint")}>
          <Textarea value={form.v("support")} onChange={(e) => form.set("support", e.target.value)} rows={2} />
        </Field>
      </div>
    </Card>
  );
}

const gatewayFields: { id: string; secret: string; sandbox?: string }[] = [
  { id: "zarinpal", secret: "zarinpal_merchant", sandbox: "zarinpal_sandbox" },
  { id: "idpay", secret: "idpay_key", sandbox: "idpay_sandbox" },
  { id: "payping", secret: "payping_token" },
];

function OnlinePay({ s, form }: { s: Record<string, string>; form: Form }) {
  const { t, lang } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const on = (k: string) => form.v(k) === "1";
  const toggle = (k: string) => (x: boolean) => form.set(k, x ? "1" : "0");

  // Removing a key happens at once: there is nothing to type, and it should
  // not wait on unrelated edits.
  const removeKey = async (secret: string, name: string) => {
    if (!(await confirm({ title: t("set.pay.remove.q", { name }), danger: true }))) return;
    try {
      await api("/settings", { method: "PATCH", body: { [secret]: "" } });
      toast.success(t("ui.saved"));
      qc.invalidateQueries({ queryKey: ["settings"] });
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <Card>
      <CardHeader title={t("set.pay")} sub={t("set.pay.sub")} icon={<CreditCard className="size-[18px]" />} />
      <div className="px-5 pb-5 space-y-3">
        <div className="rounded-2xl border border-line p-4">
          <Switch checked={on("pay_card")} onChange={toggle("pay_card")} label={<b>{gatewayNames.receipt[lang]}</b>} />
          <div className="text-xs text-muted mt-2 ms-14">{t("set.pay.card.s")}</div>
        </div>
        {gatewayFields.map((g) => {
          const isSet = s[g.secret + "_set"] === "1";
          const enabled = on("pay_" + g.id + "_on");
          return (
            <div key={g.id} className={cn("rounded-2xl border p-4 space-y-3 transition-colors", isSet && enabled ? "border-primary/40 bg-primary-soft" : "border-line")}>
              <div className="flex items-center justify-between gap-3">
                <Switch checked={enabled} onChange={toggle("pay_" + g.id + "_on")} label={<b>{gatewayNames[g.id][lang]}</b>} />
                <Badge tone={isSet && enabled ? "success" : "neutral"} dot>
                  {!isSet ? t("set.pay.nokey") : enabled ? t("set.idpay.on") : t("set.idpay.off")}
                </Badge>
              </div>
              <Field label={t("set.pay.key." + g.id)} hint={isSet ? t("set.idpay.key.set") : undefined}>
                <PasswordInput value={form.secret(g.secret)} onChange={(e) => form.setSecret(g.secret, e.target.value)} placeholder={isSet ? "••••••••••••" : ""} autoComplete="off" />
              </Field>
              <div className="flex flex-wrap items-center justify-between gap-2">
                {g.sandbox ? <Switch checked={on(g.sandbox)} onChange={toggle(g.sandbox)} label={t("set.idpay.sandbox")} /> : <span />}
                {isSet && (
                  <button className="text-xs text-danger font-semibold" onClick={() => removeKey(g.secret, gatewayNames[g.id][lang])}>{t("set.idpay.remove")}</button>
                )}
              </div>
            </div>
          );
        })}
        <Field label={t("set.idpay.unit")} hint={t("set.idpay.unit.hint")}>
          <Select value={form.v("price_unit") || "toman"} onChange={(e) => form.set("price_unit", e.target.value)}>
            <option value="toman">{t("set.idpay.toman")}</option>
            <option value="rial">{t("set.idpay.rial")}</option>
          </Select>
        </Field>
        <div className="text-xs text-muted leading-relaxed">{t("set.pay.callback")}</div>
      </div>
    </Card>
  );
}

function Security() {
  const { t } = useI18n();
  const me = useMe().data;
  const qc = useQueryClient();
  const toast = useToast();
  const [pw, setPw] = useState({ current: "", new: "" });
  const [busy, setBusy] = useState(false);
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null);
  const [code, setCode] = useState("");
  const [disabling, setDisabling] = useState(false);
  const [dpw, setDpw] = useState("");
  if (!me) return null;
  const a = me.admin;

  const changePw = async () => {
    setBusy(true);
    try {
      await api("/me/password", { body: pw });
      toast.success(t("set.password.done"));
      qc.setQueryData(["me"], null);
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };
  const start2fa = async () => {
    try {
      setSetup(await api<{ secret: string; uri: string }>("/me/totp/setup", { method: "POST" }));
    } catch (e) {
      toast.error(e);
    }
  };
  const enable2fa = async () => {
    try {
      await api("/me/totp/enable", { body: { code: latinDigits(code) } });
      toast.success(t("set.2fa.done"));
      setSetup(null);
      setCode("");
      qc.invalidateQueries({ queryKey: ["me"] });
    } catch (e) {
      toast.error(e);
    }
  };
  const disable2fa = async () => {
    try {
      await api("/me/totp/disable", { body: { password: dpw } });
      toast.success(t("set.2fa.disabled"));
      setDisabling(false);
      setDpw("");
      qc.invalidateQueries({ queryKey: ["me"] });
    } catch (e) {
      toast.error(e);
    }
  };

  return (
    <Card>
      <CardHeader title={t("set.security")} icon={<ShieldCheck className="size-[18px]" />} />
      <div className="px-5 pb-5 space-y-5">
        <div className="flex items-center gap-3 rounded-2xl border border-line p-4">
          <div className={`size-10 rounded-xl grid place-items-center ${a.totp_enabled ? "bg-success-soft text-success" : "bg-warning-soft text-warning"}`}><KeyRound className="size-5" /></div>
          <div className="flex-1">
            <div className="font-bold text-sm">{t("set.2fa")}</div>
            <Badge tone={a.totp_enabled ? "success" : "warning"} className="mt-1">{a.totp_enabled ? t("set.2fa.on") : t("set.2fa.off")}</Badge>
          </div>
          {a.totp_enabled ? <Button variant="secondary" size="sm" onClick={() => setDisabling(true)}>{t("set.2fa.disable")}</Button> : <Button size="sm" onClick={start2fa}>{t("set.2fa.enable")}</Button>}
        </div>
        <div>
          <div className="font-bold text-sm mb-3">{t("set.password")}</div>
          <div className="grid sm:grid-cols-2 gap-3">
            <Field label={t("set.password.current")}><PasswordInput value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} autoComplete="current-password" /></Field>
            <Field label={t("set.password.new")} hint={t("set.password.hint")}><PasswordInput value={pw.new} onChange={(e) => setPw({ ...pw, new: e.target.value })} autoComplete="new-password" /></Field>
          </div>
          <Button className="mt-3" variant="secondary" onClick={changePw} loading={busy} disabled={!pw.current || pw.new.length < 10}>{t("ui.save")}</Button>
        </div>
      </div>

      <Modal open={!!setup} onClose={() => setSetup(null)} title={t("set.2fa")} size="sm"
        footer={<><Button variant="secondary" onClick={() => setSetup(null)}>{t("ui.cancel")}</Button><Button onClick={enable2fa} disabled={code.length < 6}>{t("set.2fa.enable")}</Button></>}>
        {setup && (
          <div className="space-y-4 text-center">
            <p className="text-sm text-muted">{t("set.2fa.scan")}</p>
            <div className="mx-auto w-52 rounded-2xl bg-white p-3 [&_svg]:w-full [&_svg]:h-auto" dangerouslySetInnerHTML={{ __html: renderSVG(setup.uri, { border: 1 }) }} />
            <code className="block text-xs ltr break-all text-muted">{setup.secret}</code>
            <Input value={code} onChange={(e) => setCode(e.target.value)} ltr inputMode="numeric" maxLength={6} placeholder={t("set.2fa.code")} className="text-center tracking-[0.4em] text-lg font-bold" />
          </div>
        )}
      </Modal>
      <Modal open={disabling} onClose={() => setDisabling(false)} title={t("set.2fa.disable")} sub={t("set.2fa.confirm")} size="sm"
        footer={<><Button variant="secondary" onClick={() => setDisabling(false)}>{t("ui.cancel")}</Button><Button variant="danger" onClick={disable2fa}>{t("set.2fa.disable")}</Button></>}>
        <PasswordInput value={dpw} onChange={(e) => setDpw(e.target.value)} autoComplete="current-password" />
      </Modal>
    </Card>
  );
}

function Admins() {
  const { t, lang } = useI18n();
  const me = useMe().data;
  const q = useQuery({ queryKey: ["admins"], queryFn: () => api<Admin[]>("/admins") });
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [adding, setAdding] = useState(false);
  const [f, setF] = useState({ username: "", password: "" });
  const add = async () => {
    try {
      await api("/admins", { body: f });
      setAdding(false);
      setF({ username: "", password: "" });
      qc.invalidateQueries({ queryKey: ["admins"] });
    } catch (e) {
      toast.error(e);
    }
  };
  return (
    <Card>
      <CardHeader title={t("set.admins")} icon={<UsersIcon className="size-[18px]" />} action={<Button size="sm" variant="soft" icon={<Plus className="size-4" />} onClick={() => setAdding(true)}>{t("set.admins.add")}</Button>} />
      <div className="border-t border-line divide-y divide-line">
        {q.data?.map((a) => (
          <div key={a.id} className="flex items-center gap-3 px-5 py-3">
            <Avatar name={a.username} size={34} />
            <div className="flex-1 min-w-0">
              <div className="text-sm font-semibold flex items-center gap-2"><span className="ltr">{a.username}</span>{a.id === me?.admin.id && <Badge tone="primary">{t("set.admins.you")}</Badge>}{a.totp_enabled && <ShieldCheck className="size-4 text-success" />}</div>
              <div className="text-xs text-muted">{a.last_login_at ? t("set.admins.last", { when: relative(a.last_login_at, lang) }) : t("ui.never")}</div>
            </div>
            {a.id !== me?.admin.id && (
              <IconButton label={t("ui.delete")} className="hover:text-danger hover:bg-danger-soft" onClick={async () => {
                if (!(await confirm({ title: t("set.admins.delete.q", { name: a.username }), danger: true, confirm: t("ui.delete") }))) return;
                try { await api(`/admins/${a.id}`, { method: "DELETE" }); qc.invalidateQueries({ queryKey: ["admins"] }); } catch (e) { toast.error(e); }
              }}><Trash2 className="size-4" /></IconButton>
            )}
          </div>
        ))}
      </div>
      <Modal open={adding} onClose={() => setAdding(false)} title={t("set.admins.add")} size="sm"
        footer={<><Button variant="secondary" onClick={() => setAdding(false)}>{t("ui.cancel")}</Button><Button onClick={add} disabled={!f.username || f.password.length < 10}>{t("ui.add")}</Button></>}>
        <div className="space-y-3">
          <Field label={t("login.username")}><Input value={f.username} onChange={(e) => setF({ ...f, username: e.target.value })} ltr /></Field>
          <Field label={t("login.password")} hint={t("set.password.hint")}><PasswordInput value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" /></Field>
        </div>
      </Modal>
    </Card>
  );
}

function Backup() {
  const { t } = useI18n();
  const toast = useToast();
  const confirm = useConfirm();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const restore = async (file: File) => {
    if (!(await confirm({ title: t("set.backup.restore.q"), body: t("set.backup.restore.s"), danger: true }))) return;
    setBusy(true);
    const form = new FormData();
    form.append("backup", file);
    try {
      await api("/restore", { form });
      toast.info(t("set.backup.restoring"));
      setTimeout(() => location.reload(), 6000);
    } catch (e) {
      toast.error(e);
      setBusy(false);
    }
  };
  return (
    <Card>
      <CardHeader title={t("set.backup")} icon={<DatabaseBackup className="size-[18px]" />} />
      <div className="px-5 pb-5 grid sm:grid-cols-2 gap-3">
        <a href="/api/backup" className="rounded-2xl border border-line p-4 hover:bg-surface-2 transition-colors block">
          <Download className="size-5 text-primary" />
          <div className="font-bold text-sm mt-2">{t("set.backup.download")}</div>
          <div className="text-xs text-muted mt-1 leading-relaxed">{t("set.backup.download.s")}</div>
        </a>
        <button onClick={() => input.current?.click()} disabled={busy} className="rounded-2xl border border-dashed border-line-strong p-4 hover:bg-surface-2 transition-colors text-start disabled:opacity-50">
          <Upload className="size-5 text-warning" />
          <div className="font-bold text-sm mt-2">{busy ? t("set.backup.restoring") : t("set.backup.restore")}</div>
          <div className="text-xs text-muted mt-1 leading-relaxed">{t("set.backup.restore.s")}</div>
        </button>
        <input ref={input} type="file" accept=".db,application/octet-stream" className="hidden" onChange={(e) => e.target.files?.[0] && restore(e.target.files[0])} />
      </div>
    </Card>
  );
}

function System({ sys }: { sys: SettingsResp["system"] }) {
  const { t, lang } = useI18n();
  const cert = !sys.admin_domain ? t("set.system.cert.none") : sys.cert_until ? t("set.system.cert.until", { date: date(sys.cert_until, lang) }) : t("set.system.cert.pending");
  return (
    <Card>
      <CardHeader title={t("set.system")} icon={<Info className="size-[18px]" />} />
      <div className="px-5 pb-3">
        <KV k={t("set.system.version")} v={<span className="ltr">{sys.version}</span>} />
        <KV k={t("set.system.ip")} v={<span className="ltr num">{sys.public_ip}</span>} />
        <KV k={t("set.system.url")} v={<span className="ltr text-xs">{sys.admin_url}</span>} />
        <KV k={t("set.system.cert")} v={cert} />
        <KV k={t("set.system.ipv6")} v={<Badge tone={sys.ipv6_egress ? "success" : "neutral"}>{sys.ipv6_egress ? t("set.system.on") : t("set.system.off")}</Badge>} />
      </div>
    </Card>
  );
}
