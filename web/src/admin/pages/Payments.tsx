import { useState } from "react";
import { Link } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, CreditCard, ExternalLink, FileText, ImageOff, Receipt, X } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { date, num, relative } from "@shared/format";
import { Avatar, Badge, Button, Card, cn, Empty, Field, Input, PageLoader, Segmented } from "@shared/ui";
import { Modal, useToast } from "@shared/overlay";
import type { Payment } from "../types";
import { gatewayNames } from "@shared/gateways";
import { PageHeader } from "../components";

type Tab = "pending" | "approved" | "rejected" | "";

export function Payments() {
  const { t, lang } = useI18n();
  const [tab, setTab] = useState<Tab>("pending");
  const q = useQuery({ queryKey: ["payments", tab], queryFn: () => api<Payment[]>(`/payments?status=${tab}`), refetchInterval: 30_000 });
  const [open, setOpen] = useState<Payment | null>(null);
  const tones = { pending: "warning", approved: "success", rejected: "danger", failed: "danger" } as const;

  return (
    <div>
      <PageHeader title={t("pay.title")} sub={t("pay.sub")} />
      <Segmented<Tab> value={tab} onChange={setTab} className="mb-4" options={[
        { value: "pending", label: t("pay.tab.pending") },
        { value: "approved", label: t("pay.tab.approved") },
        { value: "rejected", label: t("pay.tab.rejected") },
        { value: "", label: t("pay.tab.all") },
      ]} />
      {!q.data ? (
        <PageLoader />
      ) : q.data.length === 0 ? (
        <Card><Empty icon={<Receipt className="size-6" />} title={t("pay.empty")} /></Card>
      ) : (
        <div className="grid sm:grid-cols-2 xl:grid-cols-3 gap-4">
          {q.data.map((p) => (
            <Card key={p.id} className="overflow-hidden flex flex-col animate-in">
              <button onClick={() => setOpen(p)} className="relative h-44 bg-surface-2 border-b border-line grid place-items-center overflow-hidden group">
                {p.method !== "receipt" ? (
                  <div className="flex flex-col items-center gap-2 text-primary">
                    <CreditCard className="size-10" />
                    <span className="text-sm font-bold">{gatewayNames[p.method]?.[lang] ?? p.method}</span>
                    {p.track_id && <span className="text-xs text-muted ltr">#{p.track_id}</span>}
                  </div>
                ) : p.has_receipt && p.receipt_type.startsWith("image/") ? (
                  <img src={`/api/payments/${p.id}/receipt`} alt="" loading="lazy" className="size-full object-cover group-hover:scale-105 transition-transform duration-500" />
                ) : p.has_receipt ? (
                  <FileText className="size-10 text-faint" />
                ) : (
                  <ImageOff className="size-10 text-faint" />
                )}
                <span className="absolute top-3 start-3 rounded-full bg-surface shadow-card">
                  <Badge tone={tones[p.status]}>{t("pay.status." + p.status)}</Badge>
                </span>
              </button>
              <div className="p-4 flex-1 flex flex-col">
                <div className="flex items-center gap-3">
                  <Avatar name={p.username} size={34} />
                  <div className="flex-1 min-w-0">
                    <Link to={`/users/${p.user_id}`} className="text-sm font-bold hover:text-primary ltr block text-start truncate">{p.username}</Link>
                    <div className="text-xs text-muted">{p.plan_name || "—"}</div>
                  </div>
                  <div className="text-end">
                    <div className="font-extrabold num">{num(p.amount, lang)}</div>
                    <div className="text-[11px] text-faint" title={date(p.created_at, lang, true)}>{relative(p.created_at, lang)}</div>
                  </div>
                </div>
                {p.note && <div className="text-xs text-muted mt-3 bg-surface-2 rounded-lg px-2.5 py-1.5">“{p.note}”</div>}
                {p.status === "pending" ? (
                  <Button className="mt-4" block onClick={() => setOpen(p)}>{t("pay.review")}</Button>
                ) : (
                  <div className="text-xs text-faint mt-3">{p.decided_by} · {p.decided_at ? relative(p.decided_at, lang) : ""}{p.admin_note && ` · ${p.admin_note}`}</div>
                )}
              </div>
            </Card>
          ))}
        </div>
      )}
      {open && <Review p={open} onClose={() => setOpen(null)} />}
    </div>
  );
}

function Review({ p, onClose }: { p: Payment; onClose: () => void }) {
  const { t, lang } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const [mode, setMode] = useState<"extend" | "apply" | "none">("extend");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState<"" | "ok" | "no">("");
  const url = `/api/payments/${p.id}/receipt`;

  const decide = async (ok: boolean) => {
    setBusy(ok ? "ok" : "no");
    try {
      await api(`/payments/${p.id}/${ok ? "approve" : "reject"}`, { body: ok ? { mode, note } : { note } });
      toast.success(ok ? t("pay.approved") : t("pay.rejected"));
      qc.invalidateQueries({ queryKey: ["payments"] });
      qc.invalidateQueries({ queryKey: ["me"] });
      qc.invalidateQueries({ queryKey: ["users"] });
      onClose();
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy("");
    }
  };

  return (
    <Modal open onClose={onClose} size="lg" title={t("pay.review")} sub={`${p.username} · ${p.plan_name || "—"} · ${num(p.amount, lang)}`}
      footer={p.status === "pending" ? (
        <>
          <Button variant="secondary" className="text-danger" icon={<X className="size-4" />} loading={busy === "no"} onClick={() => decide(false)}>{t("pay.reject")}</Button>
          <Button variant="success" icon={<Check className="size-4" />} loading={busy === "ok"} onClick={() => decide(true)}>{t("pay.approve")}</Button>
        </>
      ) : undefined}>
      <div className="grid md:grid-cols-2 gap-5">
        <div className="rounded-2xl bg-surface-2 border border-line overflow-hidden grid place-items-center min-h-64">
          {p.has_receipt && p.receipt_type.startsWith("image/") ? (
            <a href={url} target="_blank" rel="noreferrer"><img src={url} alt="" className="max-h-[60vh] object-contain" /></a>
          ) : p.has_receipt ? (
            <a href={url} target="_blank" rel="noreferrer" className="flex flex-col items-center gap-2 text-primary font-semibold text-sm"><FileText className="size-10" />PDF</a>
          ) : (
            <div className="text-sm text-muted p-6 text-center">{t("pay.noimage")}</div>
          )}
        </div>
        <div className="space-y-4">
          {p.has_receipt && (
            <a href={url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-sm text-primary font-semibold"><ExternalLink className="size-4" />{t("pay.open")}</a>
          )}
          {p.note && <div className="text-sm bg-surface-2 rounded-xl p-3">“{p.note}”</div>}
          {p.status === "pending" && (
            <>
              <Field label={t("pay.mode")}>
                <div className="space-y-2">
                  {(["extend", "apply", "none"] as const).map((m) => (
                    <button key={m} type="button" onClick={() => setMode(m)} className={cn("w-full text-start h-11 px-3.5 rounded-xl border text-sm font-semibold transition-colors", mode === m ? "border-primary bg-primary-soft text-primary" : "border-line hover:bg-surface-2")}>
                      {t(`pay.mode.${m}`)}
                    </button>
                  ))}
                </div>
              </Field>
              <Field label={t("pay.note")}>
                <Input value={note} onChange={(e) => setNote(e.target.value)} maxLength={300} />
              </Field>
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}
