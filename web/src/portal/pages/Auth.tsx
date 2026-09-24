import { useState, type FormEvent } from "react";
import { Link, useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ArrowRight, Wifi } from "lucide-react";
import { api, errCode } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { latinDigits } from "@shared/format";
import { Button, Card, Field, Input, LogoMark, PasswordInput } from "@shared/ui";
import { useInfo } from "../data";

function AuthCard({ title, sub, children }: { title: string; sub: string; children: React.ReactNode }) {
  return (
    <div className="max-w-md mx-auto pt-4 sm:pt-10 animate-in">
      <div className="text-center mb-6">
        <LogoMark size={52} className="mx-auto mb-4 drop-shadow-[0_10px_24px_var(--primary)]" />
        <h1 className="text-2xl font-extrabold">{title}</h1>
        <p className="text-muted mt-1.5">{sub}</p>
      </div>
      <Card className="p-6 sm:p-7">{children}</Card>
    </div>
  );
}

export function Login() {
  const { t, dir } = useI18n();
  const qc = useQueryClient();
  const nav = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await api("/login", { body: { username: latinDigits(username), password } });
      await qc.invalidateQueries();
      nav("/");
    } catch (e) {
      setErr(t("err." + errCode(e)));
    } finally {
      setBusy(false);
    }
  };
  const Arrow = dir === "rtl" ? ArrowLeft : ArrowRight;

  return (
    <AuthCard title={t("auth.login.title")} sub={t("auth.login.sub")}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("auth.username")}>
          <Input value={username} onChange={(e) => setUsername(e.target.value)} ltr autoComplete="username" autoCapitalize="none" required />
        </Field>
        <Field label={t("auth.password")}>
          <PasswordInput value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" required />
        </Field>
        {err && <div className="text-sm text-danger bg-danger-soft rounded-xl px-3.5 py-2.5">{err}</div>}
        <Button type="submit" size="lg" block loading={busy} icon={<Arrow className="size-4" />}>
          {t("auth.login")}
        </Button>
      </form>
      <p className="text-center text-sm text-muted mt-5">
        {t("auth.nohave")}{" "}
        <Link to="/signup" className="text-primary font-semibold hover:underline">
          {t("auth.signup")}
        </Link>
      </p>
    </AuthCard>
  );
}

export function Signup() {
  const { t } = useI18n();
  const info = useInfo();
  const qc = useQueryClient();
  const nav = useNavigate();
  const [f, setF] = useState({ username: "", password: "", display_name: "", phone: "" });
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value });

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await api("/signup", { body: { ...f, username: latinDigits(f.username).trim().toLowerCase() } });
      await qc.invalidateQueries();
      nav("/");
    } catch (e) {
      setErr(t("err." + errCode(e)));
    } finally {
      setBusy(false);
    }
  };

  if (info.data && !info.data.signup_open) {
    return (
      <AuthCard title={t("auth.signup.title")} sub={t("welcome.closed")}>
        <Link to="/login">
          <Button block size="lg">{t("auth.login")}</Button>
        </Link>
      </AuthCard>
    );
  }

  return (
    <AuthCard title={t("auth.signup.title")} sub={t("auth.signup.sub")}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("auth.username")} hint={t("auth.username.hint")}>
          <Input value={f.username} onChange={set("username")} ltr autoComplete="username" autoCapitalize="none" required minLength={3} maxLength={32} />
        </Field>
        <Field label={t("auth.password")} hint={t("auth.password.hint")}>
          <PasswordInput value={f.password} onChange={set("password")} autoComplete="new-password" required minLength={6} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label={t("auth.name")} hint={t("auth.name.hint")}>
            <Input value={f.display_name} onChange={set("display_name")} maxLength={60} />
          </Field>
          <Field label={t("auth.phone")}>
            <Input value={f.phone} onChange={set("phone")} ltr inputMode="tel" maxLength={20} placeholder="09…" />
          </Field>
        </div>
        {info.data?.your_ip && (
          <div className="flex items-start gap-2.5 text-[13px] text-muted bg-surface-2 rounded-xl px-3.5 py-3">
            <Wifi className="size-4 shrink-0 mt-0.5 text-primary" />
            <span>
              {t("auth.ipnote")} <b className="ltr num text-fg">{info.data.your_ip}</b>
            </span>
          </div>
        )}
        {err && <div className="text-sm text-danger bg-danger-soft rounded-xl px-3.5 py-2.5">{err}</div>}
        <Button type="submit" size="lg" block loading={busy}>
          {t("auth.signup")}
        </Button>
      </form>
      <p className="text-center text-sm text-muted mt-5">
        {t("auth.have")}{" "}
        <Link to="/login" className="text-primary font-semibold hover:underline">
          {t("auth.login")}
        </Link>
      </p>
    </AuthCard>
  );
}
