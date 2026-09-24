import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, Server, Terminal, Trash2 } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { bytes, num, rate, relative, uptime } from "@shared/format";
import { Badge, Button, Card, CopyButton, Empty, Field, IconButton, Input, PageLoader, Select } from "@shared/ui";
import { Sparkline } from "@shared/charts";
import { Modal, useConfirm, useToast } from "@shared/overlay";
import type { Gate } from "../types";
import { Meter, PageHeader } from "../components";

const INSTALL = "curl -fsSL https://raw.githubusercontent.com/amor7/gozarDNS/main/install.sh | sudo bash";

export function Gates() {
  const { t } = useI18n();
  const q = useQuery({ queryKey: ["gates"], queryFn: () => api<Gate[]>("/gates"), refetchInterval: 15_000 });
  const [adding, setAdding] = useState(false);
  const [token, setToken] = useState<string | null>(null);
  if (!q.data) return <PageLoader />;
  return (
    <div>
      <PageHeader title={t("gates.title")} sub={t("gates.sub")} actions={<Button icon={<Plus className="size-4" />} onClick={() => setAdding(true)}>{t("gates.add")}</Button>} />
      {q.data.length === 0 ? (
        <Card><Empty icon={<Server className="size-6" />} title={t("dash.nogate.t")} sub={t("dash.nogate.s")} action={<Button onClick={() => setAdding(true)}>{t("gates.add")}</Button>} /></Card>
      ) : (
        <div className="grid lg:grid-cols-2 gap-4">
          {q.data.map((g) => <GateCard key={g.id} g={g} onToken={setToken} />)}
        </div>
      )}
      {adding && <AddGate onClose={() => setAdding(false)} onToken={setToken} />}
      {token && <TokenModal token={token} onClose={() => setToken(null)} />}
    </div>
  );
}

function GateCard({ g, onToken }: { g: Gate; onToken: (t: string) => void }) {
  const { t, lang } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const metrics = useQuery({ queryKey: ["metrics", g.id], queryFn: () => api<{ ts: number; rx: number; tx: number }[]>(`/metrics?node=${g.id}&range=24h`) });
  const s = g.stats?.host;
  const st = g.stats?.stats;
  const setEnforce = async (v: string) => {
    try {
      await api(`/gates/${g.id}`, { method: "PATCH", body: { enforce: v } });
      qc.invalidateQueries({ queryKey: ["gates"] });
    } catch (e) {
      toast.error(e);
    }
  };
  return (
    <Card className="p-5 animate-in">
      <div className="flex items-start gap-3">
        <div className={`size-11 rounded-2xl grid place-items-center ${g.online ? "gradient-bg text-white" : "bg-surface-2 text-faint"}`}><Server className="size-5" /></div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-extrabold ltr">{g.name}</span>
            <Badge tone={g.online ? "success" : g.last_seen_at ? "danger" : "neutral"} dot>{g.online ? t("dash.online.badge") : g.last_seen_at ? t("dash.offline") : t("gates.never")}</Badge>
          </div>
          <div className="text-xs text-muted mt-0.5 ltr text-start num">{g.public_ip || "—"}{g.version && ` · ${g.version}`}</div>
          {g.last_seen_at && <div className="text-[11px] text-faint mt-0.5">{t("gates.lastseen", { when: relative(g.last_seen_at, lang) })}</div>}
        </div>
        <IconButton label={t("gates.token.new")} onClick={async () => {
          if (!(await confirm({ title: t("gates.token.new.q"), body: t("gates.token.new.s") }))) return;
          try { const r = await api<{ token: string }>(`/gates/${g.id}/token`, { method: "POST" }); onToken(r.token); } catch (e) { toast.error(e); }
        }}><KeyRound className="size-4" /></IconButton>
        <IconButton label={t("ui.delete")} className="hover:text-danger hover:bg-danger-soft" onClick={async () => {
          if (!(await confirm({ title: t("gates.delete.q", { name: g.name }), body: t("gates.delete.s"), danger: true, confirm: t("ui.delete") }))) return;
          try { await api(`/gates/${g.id}`, { method: "DELETE" }); qc.invalidateQueries({ queryKey: ["gates"] }); } catch (e) { toast.error(e); }
        }}><Trash2 className="size-4" /></IconButton>
      </div>

      {s && (
        <>
          <div className="grid grid-cols-2 gap-4 mt-5">
            <Meter label={t("dash.cpu")} value={`${num(s.cpu, lang, 1)}%`} pct={s.cpu} />
            <Meter label={t("dash.ram")} value={bytes(s.mem_used, lang)} pct={s.mem_total ? (s.mem_used / s.mem_total) * 100 : 0} />
          </div>
          <div className="mt-4 rounded-2xl bg-surface-2 border border-line p-3 flex items-center gap-4">
            <div className="flex-1 min-w-0">
              <div className="text-[11px] text-muted">{t("dash.net")} · 24h</div>
              <div className="text-sm font-bold num mt-0.5">↓ {rate(s.rx_bps, lang)} · ↑ {rate(s.tx_bps, lang)}</div>
            </div>
            <Sparkline values={(metrics.data || []).map((m) => m.rx)} width={140} height={40} />
          </div>
          <div className="grid grid-cols-3 gap-2 mt-3 text-center">
            <Tile label={t("dash.conns")} value={num(st?.open_conns ?? s.conns, lang)} />
            <Tile label={t("gates.queries")} value={num(st?.queries ?? 0, lang)} />
            <Tile label={t("dash.uptime")} value={uptime(s.uptime, lang)} />
          </div>
        </>
      )}

      <div className="mt-5 pt-4 border-t border-line flex flex-col sm:flex-row sm:items-center gap-2">
        <span className="text-sm font-semibold sm:flex-1">{t("gates.enforce")}</span>
        <Select value={g.enforce} onChange={(e) => setEnforce(e.target.value)} className="sm:w-60 h-10">
          <option value="auto">{t("gates.enforce.auto")}</option>
          <option value="on">{t("gates.enforce.on")}</option>
          <option value="off">{t("gates.enforce.off")}</option>
        </Select>
      </div>
      {g.enforce === "auto" && <div className="text-[11px] text-faint mt-1.5">{t("gates.enforce.auto.s")}</div>}
    </Card>
  );
}

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl bg-surface-2 py-2 px-1">
      <div className="text-[10px] text-faint truncate">{label}</div>
      <div className="text-xs font-bold mt-0.5 num">{value}</div>
    </div>
  );
}

function AddGate({ onClose, onToken }: { onClose: () => void; onToken: (t: string) => void }) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const add = async () => {
    setBusy(true);
    try {
      const r = await api<{ token: string }>("/gates", { body: { name } });
      qc.invalidateQueries({ queryKey: ["gates"] });
      onClose();
      onToken(r.token);
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open onClose={onClose} title={t("gates.add")} size="sm"
      footer={<><Button variant="secondary" onClick={onClose}>{t("ui.cancel")}</Button><Button onClick={add} loading={busy}>{t("ui.add")}</Button></>}>
      <Field label={t("gates.add.name")}>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t("gates.add.name.ph")} ltr />
      </Field>
    </Modal>
  );
}

function TokenModal({ token, onClose }: { token: string; onClose: () => void }) {
  const { t } = useI18n();
  return (
    <Modal open onClose={onClose} title={t("gates.token.title")} sub={t("gates.token.s")} footer={<Button onClick={onClose}>{t("ui.close")}</Button>}>
      <div className="space-y-4">
        <div>
          <div className="text-xs font-semibold text-muted mb-1.5 flex items-center gap-1.5"><Terminal className="size-3.5" />{t("gates.token.install")}</div>
          <div className="flex gap-2">
            <code className="flex-1 min-w-0 block rounded-xl bg-[#0d0d16] text-[#c9c6ff] px-3.5 py-3 text-xs ltr text-start overflow-x-auto whitespace-nowrap">{INSTALL}</code>
            <CopyButton text={INSTALL} label="" />
          </div>
        </div>
        <div>
          <div className="text-xs font-semibold text-muted mb-1.5 flex items-center gap-1.5"><KeyRound className="size-3.5" />{t("gates.token.title")}</div>
          <code className="block rounded-xl bg-surface-2 border border-line px-3.5 py-3 text-xs ltr text-start break-all font-mono">{token}</code>
          <CopyButton text={token} size="md" className="mt-2 w-full" />
        </div>
      </div>
    </Modal>
  );
}
