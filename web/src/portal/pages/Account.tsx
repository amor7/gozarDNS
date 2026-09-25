import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router";
import { KeyRound, LogOut, Palette } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { useTheme, type ThemePref } from "@shared/theme";
import { date } from "@shared/format";
import { Avatar, Button, Card, CardHeader, Field, PasswordInput, Segmented } from "@shared/ui";
import { useToast } from "@shared/overlay";
import { useMe } from "../data";

export function Account() {
  const { t, lang, setLang } = useI18n();
  const { pref, setPref } = useTheme();
  const me = useMe().data;
  const qc = useQueryClient();
  const nav = useNavigate();
  const toast = useToast();
  const [pw, setPw] = useState({ current: "", new: "" });
  const [busy, setBusy] = useState(false);
  if (!me) return null;
  const u = me.user;

  const changePw = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api("/password", { body: pw });
      toast.success(t("acc.changed"));
      setPw({ current: "", new: "" });
    } catch (e) {
      toast.error(e);
    } finally {
      setBusy(false);
    }
  };

  const logout = async () => {
    await api("/logout", { method: "POST" }).catch(() => {});
    await qc.invalidateQueries();
    nav("/");
  };

  return (
    <div className="space-y-4 animate-in max-w-2xl">
      <h1 className="text-2xl font-extrabold">{t("acc.title")}</h1>

      <Card className="p-5 flex items-center gap-4">
        <Avatar name={u.username} size={56} />
        <div className="flex-1 min-w-0">
          <div className="font-extrabold text-lg truncate">{u.display_name || u.username}</div>
          <div className="text-sm text-muted ltr text-start">@{u.username}</div>
          <div className="text-xs text-faint mt-1">{t("acc.member", { date: date(u.created_at, lang) })}</div>
        </div>
      </Card>

      <Card>
        <CardHeader title={t("acc.prefs")} icon={<Palette className="size-[18px]" />} />
        <div className="px-5 pb-5 space-y-4">
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-semibold">{t("acc.lang")}</span>
            <Segmented value={lang} onChange={setLang} options={[{ value: "fa", label: "فارسی" }, { value: "en", label: "English" }]} />
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-semibold">{t("acc.theme")}</span>
            <Segmented<ThemePref> value={pref} onChange={setPref} options={[
              { value: "system", label: t("acc.theme.system") },
              { value: "light", label: t("acc.theme.light") },
              { value: "dark", label: t("acc.theme.dark") },
            ]} />
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader title={t("acc.password")} icon={<KeyRound className="size-[18px]" />} />
        <form onSubmit={changePw} className="px-5 pb-5 grid sm:grid-cols-2 gap-3">
          <Field label={t("acc.current")}>
            <PasswordInput value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} autoComplete="current-password" required />
          </Field>
          <Field label={t("acc.new")} hint={t("auth.password.hint")}>
            <PasswordInput value={pw.new} onChange={(e) => setPw({ ...pw, new: e.target.value })} autoComplete="new-password" minLength={6} required />
          </Field>
          <div className="sm:col-span-2">
            <Button type="submit" loading={busy}>{t("ui.save")}</Button>
          </div>
        </form>
      </Card>


      <Button variant="secondary" block size="lg" icon={<LogOut className="size-4" />} onClick={logout} className="text-danger">
        {t("acc.logout")}
      </Button>
    </div>
  );
}
