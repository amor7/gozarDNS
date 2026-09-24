import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { renderSVG } from "uqr";
import { CreditCard, DatabaseBackup, Download, Info, KeyRound, Plus, ShieldCheck, SlidersHorizontal, Trash2, Upload, Users as UsersIcon } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { date, latinDigits, relative } from "@shared/format";
import { Avatar, Badge, Button, Card, CardHeader, Field, IconButton, Input, PageLoader, PasswordInput, Select, Switch, Textarea } from "@shared/ui";
import { Modal, useConfirm, useToast } from "@shared/overlay";
import type { Admin, Plan } from "../types";
import { KV, PageHeader } from "../components";
import { useMe } from "../App";

type SettingsResp = {
  settings: Record<string, string>;
  system: { version: string; public_ip: string; admin_url: string; admin_domain: string; cert_until: number; ipv6_egress: boolean; control_port: number; fingerprint: string };
};

export function Settings() {
  const { t } = useI18n();
  const q = useQuery({ queryKey: ["settings"], queryFn: () => api<SettingsResp>("/settings") });
  if (!q.data) return <PageLoader />;
  return (
    <div className="space-y-5">
      <PageHeader title={t("set.title")} />
      <div className="grid xl:grid-cols-2 gap-5 items-start">
        <div className="space-y-5">
          <General s={q.data.settings} />
          <OnlinePay s={q.data.settings} />
          <Backup />
        </div>
        <div className="space-y-5">
          <Security />
          <Admins />
          <System sys={q.data.system} />
        </div>
      </div>
    </div>
  );
}

function General({ s }: { s: Record<string, string> }) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const plans = useQuery({ queryKey: ["plans"], queryFn: () => api<Plan[]>("/plans") });
  const [f, setF] = useState(s);
  const [busy, setBusy] = useState(false);
  useEffect(() => setF(s), [s]);
  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setF({ ...f, [k]: e.target.value });
  const save = async () => {
    setBusy(true);
    try {
      await api("/settings", { method: "PATCH", body: { ...f, dns_address: latinDigits(f.dns_address || "") } });
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
    <Card>
      <CardHeader title={t("set.general")} icon={<SlidersHorizontal className="size-[18px]" />} />
      <div className="px-5 pb-5 space-y-4">
        <div className="grid sm:grid-cols-3 gap-3">
          <Field label={t("set.brand")} hint={t("set.brand.hint")} className="sm:col-span-2"><Input value={f.brand || ""} onChange={set("brand")} /></Field>
          <Field label={t("set.currency")}><Input value={f.currency || ""} onChange={set("currency")} /></Field>
        </div>
        <Field label={t("set.support")}><Textarea value={f.support || ""} onChange={set("support")} rows={2} placeholder={t("set.support.ph")} /></Field>
        <Field label={t("set.notice")} hint={t("set.notice.hint")}><Textarea value={f.notice || ""} onChange={set("notice")} rows={2} /></Field>
        <Field label={t("set.payment")} hint={t("set.payment.hint")}><Textarea value={f.payment_info || ""} onChange={set("payment_info")} rows={3} /></Field>
        <div className="rounded-2xl border border-line p-4 space-y-3">
          <div className="font-bold text-sm">{t("set.signup")}</div>
          <Switch checked={f.signup_open === "1"} onChange={(v) => setF({ ...f, signup_open: v ? "1" : "0" })} label={t("set.signup.open")} />
          <Field label={t("set.signup.plan")}>
            <Select value={f.signup_plan || "0"} onChange={set("signup_plan")}>
              <option value="0">{t("set.signup.plan.none")}</option>
              {plans.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </Select>
          </Field>
        </div>
        <Field label={t("set.dns")} hint={t("set.dns.hint")}><Input value={f.dns_address || ""} onChange={set("dns_address")} ltr placeholder="—" /></Field>
        <Button onClick={save} loading={busy}>{t("ui.save")}</Button>
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

function OnlinePay({ s }: { s: Record<string, string> }) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [key, setKey] = useState("");
  const [sandbox, setSandbox] = useState(s.idpay_sandbox === "1");
  const [unit, setUnit] = useState(s.price_unit || "toman");
  const [busy, setBusy] = useState(false);
  const isSet = s.idpay_key_set === "1";
  const save = async (extra: Record<string, string> = {}) => {
    setBusy(true);
    try {
      const body: Record<string, string> = { idpay_sandbox: sandbox ? "1" : "0", price_unit: unit, ...extra };
      if (key.trim()) body.idpay_key = key.trim();
      await api("/settings", { method: "PATCH", body });
      setKey("");
      toast.success(t("ui.saved"));
      qc.invalidateQueries({ queryKey: ["settings"] });
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Card>
      <CardHeader title={t("set.idpay")} sub={t("set.idpay.sub")} icon={<CreditCard className="size-[18px]" />}
        action={<Badge tone={isSet ? "success" : "neutral"} dot>{isSet ? t("set.idpay.on") : t("set.idpay.off")}</Badge>} />
      <div className="px-5 pb-5 space-y-4">
        <Field label={t("set.idpay.key")} hint={isSet ? t("set.idpay.key.set") : t("set.idpay.key.hint")}>
          <PasswordInput value={key} onChange={(e) => setKey(e.target.value)} placeholder={isSet ? "••••••••••••" : ""} autoComplete="off" />
        </Field>
        <Field label={t("set.idpay.unit")} hint={t("set.idpay.unit.hint")}>
          <Select value={unit} onChange={(e) => setUnit(e.target.value)}>
            <option value="toman">{t("set.idpay.toman")}</option>
            <option value="rial">{t("set.idpay.rial")}</option>
          </Select>
        </Field>
        <Switch checked={sandbox} onChange={setSandbox} label={t("set.idpay.sandbox")} />
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => save()} loading={busy}>{t("ui.save")}</Button>
          {isSet && (
            <Button variant="secondary" className="text-danger" onClick={async () => {
              if (await confirm({ title: t("set.idpay.remove.q"), danger: true })) save({ idpay_key: "" });
            }}>{t("set.idpay.remove")}</Button>
          )}
        </div>
      </div>
    </Card>
  );
}
