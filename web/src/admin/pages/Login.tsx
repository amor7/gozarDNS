import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { KeyRound, Languages, Moon, ShieldCheck, Sun } from "lucide-react";
import { api, errCode } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { useTheme } from "@shared/theme";
import { latinDigits } from "@shared/format";
import { Button, Card, Field, IconButton, Input, LogoMark, PasswordInput } from "@shared/ui";

export function Login() {
  const { t, lang, setLang } = useI18n();
  const { dark, toggle } = useTheme();
  const qc = useQueryClient();
  const [f, setF] = useState({ username: "", password: "", code: "" });
  const [needCode, setNeedCode] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await api("/login", { body: { ...f, code: latinDigits(f.code) } });
      await qc.invalidateQueries({ queryKey: ["me"] });
    } catch (e) {
      const code = errCode(e);
      if (code === "totp_required") setNeedCode(true);
      else setErr(t("err." + code));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="min-h-dvh grid lg:grid-cols-2">
      <div className="relative hidden lg:flex flex-col justify-between p-12 overflow-hidden gradient-bg text-white">
        <div className="absolute inset-0 opacity-20" style={{ backgroundImage: "radial-gradient(circle at 20% 20%, white 1px, transparent 1px)", backgroundSize: "28px 28px" }} />
        <div className="absolute -bottom-40 -end-40 size-[520px] rounded-full bg-white/10 blur-3xl" />
        <div className="relative flex items-center gap-3">
          <svg width="40" height="40" viewBox="0 0 40 40"><rect x="1" y="1" width="38" height="38" rx="12" fill="white" fillOpacity=".18" /><path d="M12 28V19a8 8 0 0 1 16 0v9" fill="none" stroke="white" strokeWidth="3.2" strokeLinecap="round" /><path d="M17 28v-8.5a3 3 0 0 1 6 0V28" fill="none" stroke="white" strokeOpacity=".6" strokeWidth="2.6" strokeLinecap="round" /><circle cx="20" cy="11.5" r="2.4" fill="white" /></svg>
          <span className="text-xl font-extrabold">Gozar</span>
        </div>
        <div className="relative">
          <h2 className="text-4xl font-black leading-tight">{lang === "fa" ? "همه‌چیز سرویس،" : "Your whole service,"}<br />{lang === "fa" ? "در یک نگاه." : "at a glance."}</h2>
          <p className="mt-4 text-white/80 max-w-md leading-relaxed">
            {lang === "fa" ? "مشتری‌ها، پلن‌ها، رسیدها و سلامت سرورها — بدون خط فرمان." : "Customers, plans, receipts and server health — no command line needed."}
          </p>
        </div>
        <div className="relative flex items-center gap-2 text-white/70 text-sm">
          <ShieldCheck className="size-4" />
          {lang === "fa" ? "رمزها فقط به‌صورت هش ذخیره می‌شوند." : "Passwords are only ever stored hashed."}
        </div>
      </div>

      <div className="flex flex-col">
        <div className="flex justify-end gap-1 p-4">
          <IconButton label="lang" onClick={() => setLang(lang === "fa" ? "en" : "fa")}><Languages className="size-[18px]" /></IconButton>
          <IconButton label="theme" onClick={toggle}>{dark ? <Sun className="size-[18px]" /> : <Moon className="size-[18px]" />}</IconButton>
        </div>
        <div className="flex-1 grid place-items-center px-4 pb-16">
          <div className="w-full max-w-sm animate-in">
            <LogoMark size={48} className="lg:hidden mb-6" />
            <h1 className="text-2xl font-extrabold">{t("login.title")}</h1>
            <p className="text-muted mt-1.5 mb-6">{t("login.sub")}</p>
            <Card className="p-6">
              <form onSubmit={submit} className="space-y-4">
                <Field label={t("login.username")}>
                  <Input value={f.username} onChange={(e) => setF({ ...f, username: e.target.value })} ltr autoComplete="username" autoCapitalize="none" required />
                </Field>
                <Field label={t("login.password")}>
                  <PasswordInput value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} autoComplete="current-password" required />
                </Field>
                {needCode && (
                  <Field label={t("login.code")} hint={t("login.code.hint")}>
                    <Input value={f.code} onChange={(e) => setF({ ...f, code: e.target.value })} ltr inputMode="numeric" autoComplete="one-time-code" maxLength={6} autoFocus className="tracking-[0.5em] text-center text-lg font-bold" />
                  </Field>
                )}
                {err && <div className="text-sm text-danger bg-danger-soft rounded-xl px-3.5 py-2.5">{err}</div>}
                <Button type="submit" size="lg" block loading={busy} icon={<KeyRound className="size-4" />}>
                  {t("login.submit")}
                </Button>
              </form>
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
}
