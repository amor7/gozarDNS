import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Search, UserPlus, Users as UsersIcon } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { bytes, daysLeft, num, relative } from "@shared/format";
import { Avatar, Button, Card, CopyButton, Empty, Field, Input, PasswordInput, Progress, Select, Skeleton } from "@shared/ui";
import { Modal, useToast } from "@shared/overlay";
import type { Plan, User } from "../types";
import { PageHeader, StateBadge } from "../components";
import { UserDrawer } from "./UserDrawer";

const states = ["active", "no_plan", "expired", "over_quota", "disabled"] as const;

export function Users() {
  const { t, lang } = useI18n();
  const { id } = useParams();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState(params.get("q") || "");
  const [debounced, setDebounced] = useState(q);
  const state = params.get("state") || "";
  const plan = params.get("plan") || "";
  const sort = params.get("sort") || "recent";
  const [limit, setLimit] = useState(50);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    const h = setTimeout(() => setDebounced(q), 250);
    return () => clearTimeout(h);
  }, [q]);

  const set = (k: string, v: string) => {
    const p = new URLSearchParams(params);
    if (v) p.set(k, v);
    else p.delete(k);
    setParams(p, { replace: true });
  };

  const users = useQuery({
    queryKey: ["users", debounced, state, plan, sort, limit],
    queryFn: () => api<{ users: User[]; total: number }>(`/users?q=${encodeURIComponent(debounced)}&state=${state}&plan=${plan}&sort=${sort}&limit=${limit}`),
    placeholderData: keepPreviousData,
  });
  const plans = useQuery({ queryKey: ["plans"], queryFn: () => api<Plan[]>("/plans") });

  return (
    <div>
      <PageHeader
        title={t("users.title")}
        sub={users.data ? t("users.sub", { n: num(users.data.total, lang) }) : " "}
        actions={<Button icon={<UserPlus className="size-4" />} onClick={() => setCreating(true)}>{t("users.new")}</Button>}
      />

      <Card className="p-3 mb-4 flex flex-col lg:flex-row gap-2">
        <div className="relative flex-1">
          <Search className="size-4 absolute top-1/2 -translate-y-1/2 start-3.5 text-faint" />
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("users.search")} className="ps-10" />
        </div>
        <div className="grid grid-cols-3 gap-2 lg:w-[540px]">
          <Select value={state} onChange={(e) => set("state", e.target.value)}>
            <option value="">{t("users.filter.state")}: {t("ui.all")}</option>
            {states.map((s) => <option key={s} value={s}>{t("state." + s)}</option>)}
          </Select>
          <Select value={plan} onChange={(e) => set("plan", e.target.value)}>
            <option value="">{t("users.filter.plan")}: {t("ui.all")}</option>
            {plans.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
          <Select value={sort} onChange={(e) => set("sort", e.target.value)}>
            {["recent", "usage", "expiry", "seen", "name"].map((s) => <option key={s} value={s}>{t("users.sort." + s)}</option>)}
          </Select>
        </div>
      </Card>

      {/* State chips for one-tap filtering */}
      <div className="flex gap-2 overflow-x-auto pb-3 -mx-1 px-1 [scrollbar-width:none]">
        {["", ...states].map((s) => (
          <button key={s || "all"} onClick={() => set("state", s)}
            className={`shrink-0 h-8 px-3.5 rounded-full text-[13px] font-semibold border transition-colors ${state === s ? "gradient-bg text-white border-transparent" : "bg-surface border-line text-muted hover:text-fg"}`}>
            {s ? t("state." + s) : t("ui.all")}
          </button>
        ))}
      </div>

      <Card className="overflow-hidden">
        {users.isLoading ? (
          <div className="p-4 space-y-3">{Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="h-14" />)}</div>
        ) : users.data?.users.length === 0 ? (
          <Empty icon={<UsersIcon className="size-6" />} title={t("users.empty")} sub={t("users.empty.sub")} action={<Button icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>{t("users.new")}</Button>} />
        ) : (
          <>
            {/* Desktop table */}
            <table className="hidden md:table w-full text-sm">
              <thead>
                <tr className="text-xs text-muted border-b border-line">
                  <th className="text-start font-semibold px-5 py-3">{t("users.col.user")}</th>
                  <th className="text-start font-semibold px-3 py-3">{t("users.col.plan")}</th>
                  <th className="text-start font-semibold px-3 py-3 w-56">{t("users.col.usage")}</th>
                  <th className="text-start font-semibold px-3 py-3">{t("users.col.expires")}</th>
                  <th className="text-center font-semibold px-3 py-3">{t("users.col.devices")}</th>
                  <th className="text-end font-semibold px-5 py-3">{t("users.col.state")}</th>
                </tr>
              </thead>
              <tbody>
                {users.data?.users.map((u) => (
                  <tr key={u.id} onClick={() => navigate(`/users/${u.id}?${params}`)} className="border-b border-line last:border-0 hover:bg-surface-2 cursor-pointer transition-colors">
                    <td className="px-5 py-3">
                      <div className="flex items-center gap-3">
                        <Avatar name={u.username} size={36} />
                        <div className="min-w-0">
                          <div className="font-semibold truncate">{u.display_name || u.username}</div>
                          <div className="text-xs text-muted ltr text-start">@{u.username}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-3 py-3 text-muted">{u.plan_name || "—"}</td>
                    <td className="px-3 py-3"><UsageCell u={u} /></td>
                    <td className="px-3 py-3"><ExpiryCell u={u} /></td>
                    <td className="px-3 py-3 text-center num">{num(u.devices, lang)}<span className="text-faint">/{num(u.max_devices, lang)}</span></td>
                    <td className="px-5 py-3 text-end"><StateBadge state={u.state} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
            {/* Mobile cards */}
            <div className="md:hidden divide-y divide-line">
              {users.data?.users.map((u) => (
                <button key={u.id} onClick={() => navigate(`/users/${u.id}?${params}`)} className="w-full text-start p-4 hover:bg-surface-2">
                  <div className="flex items-center gap-3">
                    <Avatar name={u.username} size={40} />
                    <div className="flex-1 min-w-0">
                      <div className="font-semibold truncate">{u.display_name || u.username}</div>
                      <div className="text-xs text-muted">{u.plan_name || "—"}</div>
                    </div>
                    <StateBadge state={u.state} />
                  </div>
                  <div className="mt-3"><UsageCell u={u} /></div>
                </button>
              ))}
            </div>
            {users.data && users.data.users.length < users.data.total && (
              <div className="p-3 text-center border-t border-line">
                <Button variant="ghost" onClick={() => setLimit(limit + 50)}>{t("ui.loadmore")}</Button>
              </div>
            )}
          </>
        )}
      </Card>

      {id && <UserDrawer id={Number(id)} onClose={() => navigate(`/users?${params}`)} />}
      {creating && <CreateUser plans={plans.data || []} onClose={() => setCreating(false)} />}
    </div>
  );
}

function UsageCell({ u }: { u: User }) {
  const { t, lang } = useI18n();
  return (
    <div>
      <div className="flex justify-between text-xs mb-1.5">
        <span className="font-semibold num">{bytes(u.used_bytes, lang)}</span>
        <span className="text-faint num">{u.quota_bytes ? bytes(u.quota_bytes, lang) : t("ui.unlimited")}</span>
      </div>
      <Progress value={u.used_bytes} max={u.quota_bytes || 0} />
    </div>
  );
}

function ExpiryCell({ u }: { u: User }) {
  const { t, lang } = useI18n();
  if (!u.expires_at) return <span className="text-faint">∞</span>;
  const d = daysLeft(u.expires_at) ?? 0;
  if (d === 0) return <span className="text-danger font-semibold text-xs">{t("users.ended")}</span>;
  return (
    <div>
      <div className={`font-semibold num ${d <= 3 ? "text-warning" : ""}`}>{t("users.days", { n: num(d, lang) })}</div>
      {u.last_seen_at && <div className="text-[11px] text-faint">{t("users.seen", { when: relative(u.last_seen_at, lang) })}</div>}
    </div>
  );
}

function CreateUser({ plans, onClose }: { plans: Plan[]; onClose: () => void }) {
  const { t } = useI18n();
  const toast = useToast();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const def = useMemo(() => plans.find((p) => p.is_default)?.id ?? 0, [plans]);
  const [f, setF] = useState({ username: "", password: "", display_name: "", phone: "", plan_id: def, ip: "", note: "" });
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<{ user: User; password: string } | null>(null);

  const submit = async () => {
    setBusy(true);
    try {
      const res = await api<{ user: User; password: string }>("/users", { body: { ...f, plan_id: Number(f.plan_id) } });
      qc.invalidateQueries({ queryKey: ["users"] });
      setDone({ user: res.user, password: res.password || f.password });
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    const text = `${t("user.new.username")}: ${done.user.username}\n${t("user.new.password")}: ${done.password}`;
    return (
      <Modal open onClose={onClose} title={t("user.new.created")} sub={t("user.new.created.s")} size="sm"
        footer={<><CopyButton text={text} size="md" /><Button onClick={() => { onClose(); navigate(`/users/${done.user.id}`); }}>{t("ui.edit")}</Button></>}>
        <div className="rounded-2xl bg-surface-2 border border-line p-4 space-y-2 ltr text-start font-mono text-sm">
          <div><span className="text-muted">username </span><b>{done.user.username}</b></div>
          <div><span className="text-muted">password </span><b>{done.password}</b></div>
        </div>
      </Modal>
    );
  }

  return (
    <Modal open onClose={onClose} title={t("user.new.title")}
      footer={<><Button variant="secondary" onClick={onClose}>{t("ui.cancel")}</Button><Button onClick={submit} loading={busy} disabled={!f.username}>{t("users.new")}</Button></>}>
      <div className="grid sm:grid-cols-2 gap-4">
        <Field label={t("user.new.username")}>
          <Input value={f.username} onChange={(e) => setF({ ...f, username: e.target.value })} ltr autoCapitalize="none" />
        </Field>
        <Field label={t("user.new.password")} hint={t("user.new.password.hint")}>
          <PasswordInput value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="new-password" />
        </Field>
        <Field label={t("user.name")}>
          <Input value={f.display_name} onChange={(e) => setF({ ...f, display_name: e.target.value })} />
        </Field>
        <Field label={t("user.phone")}>
          <Input value={f.phone} onChange={(e) => setF({ ...f, phone: e.target.value })} ltr inputMode="tel" />
        </Field>
        <Field label={t("user.new.plan")}>
          <Select value={f.plan_id} onChange={(e) => setF({ ...f, plan_id: Number(e.target.value) })}>
            <option value={0}>{t("user.new.plan.none")}</option>
            {plans.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
        </Field>
        <Field label={t("user.new.ip")}>
          <Input value={f.ip} onChange={(e) => setF({ ...f, ip: e.target.value })} ltr placeholder="5.120.44.19" />
        </Field>
        <Field label={t("user.note")} hint={t("user.note.hint")} className="sm:col-span-2">
          <Input value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} />
        </Field>
      </div>
    </Modal>
  );
}
