import { useEffect, useRef, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { Check, CreditCard, FileText, ImageUp, Receipt, ShieldCheck } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { bytes, date, money, num, speed } from "@shared/format";
import { Badge, Button, Card, CardHeader, CopyButton, cn, Empty, Field, Input, Lines, ServiceDot } from "@shared/ui";
import { Modal, useToast } from "@shared/overlay";
import { useInfo, useMe, useRefresh } from "../data";
import { gatewayNames } from "@shared/gateways";
import type { PublicPlan } from "../types";

export function PlanCard({ plan, currency, current, action }: { plan: PublicPlan; currency: string; current?: boolean; action?: ReactNode }) {
  const { t, lang } = useI18n();
  const accent = plan.color || "var(--primary)";
  const features = [
    plan.quota_bytes ? t("plans.quota", { v: bytes(plan.quota_bytes, lang) }) : t("plans.quota.unlimited"),
    plan.duration_days ? t("plans.days", { n: num(plan.duration_days, lang) }) : t("plans.forever"),
    t("plans.devices", { n: num(plan.max_devices, lang) }),
    plan.speed_kbps ? t("plans.speed", { v: speed(plan.speed_kbps, lang) }) : t("plans.speed.unlimited"),
  ];
  if (plan.renewal === "reset" && plan.duration_days) features.push(t("plans.reset"));
  return (
    <Card className={cn("relative p-5 sm:p-6 flex flex-col overflow-hidden transition-transform hover:-translate-y-0.5", current && "ring-2 ring-primary")}>
      <div className="absolute inset-x-0 top-0 h-1" style={{ background: accent }} />
      <div className="absolute -top-16 -end-16 size-40 rounded-full blur-3xl opacity-20 pointer-events-none" style={{ background: accent }} />
      <div className="flex items-start justify-between gap-2">
        <div>
          <div className="font-extrabold text-lg">{plan.name}</div>
          {plan.description && <div className="text-[13px] text-muted mt-1 leading-relaxed">{plan.description}</div>}
        </div>
        {current && <Badge tone="primary">{t("plans.current")}</Badge>}
      </div>
      <div className="mt-5 flex items-baseline gap-1.5">
        {plan.price > 0 ? (
          <>
            <span className="text-3xl font-black num tracking-tight">{num(plan.price, lang)}</span>
            <span className="text-sm text-muted">{currency}</span>
          </>
        ) : (
          <span className="text-3xl font-black">{t("plans.free")}</span>
        )}
      </div>
      <ul className="mt-5 space-y-2.5 text-sm">
        {features.map((f) => (
          <li key={f} className="flex items-center gap-2.5">
            <span className="size-5 rounded-full grid place-items-center shrink-0" style={{ background: `color-mix(in oklab, ${accent} 18%, transparent)`, color: accent }}>
              <Check className="size-3.5" strokeWidth={3} />
            </span>
            {f}
          </li>
        ))}
      </ul>
      {plan.services.length > 0 && (
        <div className="mt-5 flex flex-wrap gap-1.5">
          {plan.services.slice(0, 12).map((s) => (
            <ServiceDot key={s.id} name={s.name.en} color={s.color} size={24} />
          ))}
          {plan.services.length > 12 && <span className="h-6 px-2 rounded-lg bg-surface-2 text-xs text-muted grid place-items-center num">+{num(plan.services.length - 12, lang)}</span>}
        </div>
      )}
      {action && <div className="mt-6 pt-1 mt-auto">{action}</div>}
    </Card>
  );
}

export function Plans() {
  const { t, lang } = useI18n();
  const info = useInfo().data;
  const me = useMe().data;
  const [buying, setBuying] = useState<PublicPlan | null>(null);
  const [params, setParams] = useSearchParams();
  const toast = useToast();
  const refresh = useRefresh();
  // Back from the payment gateway.
  useEffect(() => {
    const r = params.get("pay");
    if (!r) return;
    if (r === "ok") toast.success(t("pay.online.ok"));
    else toast.error({ code: r === "error" ? "hub_unreachable" : "pay_failed" });
    refresh();
    setParams({}, { replace: true });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [params]);
  if (!info || !me) return null;
  const statusTone = { pending: "warning", approved: "success", rejected: "danger", failed: "danger" } as const;

  return (
    <div className="space-y-6 animate-in">
      <div>
        <h1 className="text-2xl font-extrabold">{t("plans.title")}</h1>
        <p className="text-muted mt-1">{t("plans.sub")}</p>
      </div>

      {info.plans.length === 0 ? (
        <Card><Empty icon={<CreditCard className="size-6" />} title={t("plans.none")} /></Card>
      ) : (
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {info.plans.map((p) => {
            const current = me.plan?.id === p.id;
            return (
              <PlanCard key={p.id} plan={p} currency={info.currency} current={current}
                action={<Button block variant={current ? "primary" : "soft"} onClick={() => setBuying(p)}>{current ? t("plans.renew") : t("plans.buy")}</Button>} />
            );
          })}
        </div>
      )}

      {me.payments.length > 0 && (
        <Card>
          <CardHeader title={t("pay.history")} icon={<Receipt className="size-[18px]" />} />
          <div className="px-3 pb-3">
            {me.payments.map((p) => (
              <div key={p.id} className="flex items-center gap-3 p-3 rounded-2xl hover:bg-surface-2">
                <div className="flex-1 min-w-0">
                  <div className="font-semibold text-sm">{p.plan_name || "—"}</div>
                  <div className="text-xs text-muted mt-0.5">
                    {date(p.created_at, lang, true)}
                    {p.method !== "receipt" && " · " + (gatewayNames[p.method]?.[lang] ?? p.method)}
                    {p.track_id && <span className="ltr"> · #{p.track_id}</span>}
                  </div>
                  {p.admin_note && <div className="text-xs text-muted mt-1">“{p.admin_note}”</div>}
                </div>
                <div className="text-end">
                  <div className="text-sm font-bold num">{money(p.amount, lang, info.currency)}</div>
                  <Badge tone={statusTone[p.status]} className="mt-1">{t("pay.status." + p.status)}</Badge>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}

      {buying && <PaySheet plan={buying} onClose={() => setBuying(null)} />}
    </div>
  );
}

function PaySheet({ plan, onClose }: { plan: PublicPlan; onClose: () => void }) {
  const { t, lang } = useI18n();
  const info = useInfo().data!;
  const toast = useToast();
  const refresh = useRefresh();
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [paying, setPaying] = useState("");
  const online = info.gateways || [];
  const [showReceipt, setShowReceipt] = useState(online.length === 0 && info.card_pay);

  const payOnline = async (gateway: string) => {
    setPaying(gateway);
    try {
      const r = await api<{ link: string }>("/pay/online", { body: { plan_id: plan.id, gateway } });
      window.location.href = r.link;
    } catch (e) {
      toast.error(e);
      setPaying("");
    }
  };

  const pick = (f: File | null) => {
    setFile(f);
    if (preview) URL.revokeObjectURL(preview);
    setPreview(f && f.type.startsWith("image/") ? URL.createObjectURL(f) : null);
  };

  const send = async () => {
    if (!file) return;
    setBusy(true);
    const form = new FormData();
    form.append("plan_id", String(plan.id));
    form.append("amount", String(plan.price));
    form.append("note", note);
    form.append("receipt", file);
    try {
      await api("/payments", { form });
      toast.success(t("pay.sent"));
      refresh();
      onClose();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={t("pay.title", { plan: plan.name })}
      footer={showReceipt ? <><Button variant="secondary" onClick={onClose}>{t("ui.cancel")}</Button><Button onClick={send} loading={busy} disabled={!file}>{t("pay.send")}</Button></> : undefined}>
      {online.length > 0 && (
        <div className="mb-5">
          <div className="rounded-2xl gradient-bg text-white p-5 relative overflow-hidden">
            <div className="absolute -top-10 -end-10 size-32 rounded-full bg-white/10" />
            <div className="text-sm text-white/80">{t("pay.amount")}</div>
            <div className="text-3xl font-black num mt-1">{money(plan.price, lang, info.currency)}</div>
            <div className="text-sm text-white/90 font-semibold mt-4 mb-2">{online.length > 1 ? t("pay.online.choose") : t("pay.online")}</div>
            <div className="space-y-2">
              {online.map((g) => (
                <button
                  key={g}
                  onClick={() => payOnline(g)}
                  disabled={!!paying}
                  className="w-full h-12 rounded-xl bg-white text-[#3b2bb3] font-bold flex items-center justify-center gap-2 hover:brightness-95 active:scale-[0.98] transition disabled:opacity-70"
                >
                  <CreditCard className="size-5" />
                  {paying === g ? "…" : t("pay.via", { name: gatewayNames[g]?.[lang] ?? g })}
                </button>
              ))}
            </div>
            <div className="flex items-center justify-center gap-1.5 text-xs text-white/80 mt-3">
              <ShieldCheck className="size-3.5" />
              {t("pay.online.sub")}
            </div>
          </div>
          {info.card_pay && !showReceipt && (
            <button className="w-full text-center text-sm text-muted hover:text-fg mt-4" onClick={() => setShowReceipt(true)}>
              {t("pay.or.receipt")}
            </button>
          )}
        </div>
      )}
      {online.length === 0 && !info.card_pay && (
        <div className="text-sm text-muted text-center py-6">{t("pay.none")}</div>
      )}
      {showReceipt && (
      <div className="space-y-5">
        <Step n={1} title={t("pay.step1")}>
          <div className="rounded-2xl bg-surface-2 border border-line p-4">
            <div className="flex items-center justify-between">
              <span className="text-sm text-muted">{t("pay.amount")}</span>
              <span className="text-xl font-black num">{money(plan.price, lang, info.currency)}</span>
            </div>
            {info.payment_info ? (
              <div className="mt-3 pt-3 border-t border-line flex items-start gap-3">
                <Lines text={info.payment_info} className="flex-1 text-sm leading-relaxed space-y-0.5 num" />
                <CopyButton text={firstNumber(info.payment_info)} />
              </div>
            ) : (
              <div className="mt-3 text-sm text-muted">{t("pay.info.none")}</div>
            )}
          </div>
        </Step>
        <Step n={2} title={t("pay.step2")}>
          <input ref={input} type="file" accept="image/*,application/pdf" className="hidden" onChange={(e) => pick(e.target.files?.[0] ?? null)} />
          <button type="button" onClick={() => input.current?.click()}
            onDragOver={(e) => e.preventDefault()} onDrop={(e) => { e.preventDefault(); pick(e.dataTransfer.files?.[0] ?? null); }}
            className={cn("w-full rounded-2xl border-2 border-dashed transition-colors grid place-items-center text-center overflow-hidden",
              file ? "border-primary/50 bg-primary-soft p-3" : "border-line-strong hover:border-primary hover:bg-primary-soft py-8 px-4")}>
            {preview ? (
              <img src={preview} alt="" className="max-h-56 rounded-xl object-contain" />
            ) : file ? (
              <div className="flex items-center gap-2 text-sm font-semibold"><FileText className="size-5 text-primary" />{file.name}</div>
            ) : (
              <>
                <ImageUp className="size-8 text-primary mb-2" />
                <div className="font-semibold text-sm">{t("pay.drop")}</div>
                <div className="text-xs text-muted mt-1">{t("pay.drop.sub")}</div>
              </>
            )}
          </button>
          {file && <button className="text-xs text-primary font-semibold mt-2" onClick={() => input.current?.click()}>{t("pay.change")}</button>}
          <Field label={t("pay.note")} className="mt-4">
            <Input value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("pay.note.ph")} maxLength={300} />
          </Field>
        </Step>
      </div>
      )}
    </Modal>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: ReactNode }) {
  const { lang } = useI18n();
  return (
    <div>
      <div className="flex items-center gap-2.5 mb-3">
        <span className="size-6 rounded-full gradient-bg text-white text-xs font-bold grid place-items-center num">{num(n, lang)}</span>
        <span className="font-bold text-[15px]">{title}</span>
      </div>
      {children}
    </div>
  );
}

/** The longest digit run in the payment text - almost always the card number. */
function firstNumber(s: string) {
  const runs = s.replace(/[\s-]/g, "").match(/\d{8,}/g);
  return runs ? runs.sort((a, b) => b.length - a.length)[0] : s;
}
