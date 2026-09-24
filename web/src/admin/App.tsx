import { useEffect, useMemo, useRef, useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity as ActivityIcon, CornerDownLeft, Globe, Languages, LayoutDashboard, LogOut, Menu, Moon, Package, Receipt, Search, Server, Settings as SettingsIcon, Sun, Users as UsersIcon, X, type LucideIcon,
} from "lucide-react";
import { api, setUnauthorizedHandler } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { useTheme } from "@shared/theme";
import { Avatar, cn, IconButton, LogoMark, PageLoader } from "@shared/ui";
import type { Admin, User } from "./types";
import { Login } from "./pages/Login";
import { Dashboard } from "./pages/Dashboard";
import { Users } from "./pages/Users";
import { Plans } from "./pages/Plans";
import { Domains } from "./pages/Domains";
import { Payments } from "./pages/Payments";
import { Gates } from "./pages/Gates";
import { ActivityPage } from "./pages/Activity";
import { Settings } from "./pages/Settings";

type Me = { admin: Admin; brand: string; version: string; pending_payments: number };

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => api<Me>("/me").catch((e) => (e?.status === 401 ? null : Promise.reject(e))),
    refetchInterval: 30_000,
  });
}

const nav: { to: string; key: string; icon: LucideIcon; badge?: "payments" }[] = [
  { to: "/", key: "nav.dashboard", icon: LayoutDashboard },
  { to: "/users", key: "nav.users", icon: UsersIcon },
  { to: "/payments", key: "nav.payments", icon: Receipt, badge: "payments" },
  { to: "/plans", key: "nav.plans", icon: Package },
  { to: "/domains", key: "nav.domains", icon: Globe },
  { to: "/gates", key: "nav.gates", icon: Server },
  { to: "/activity", key: "nav.activity", icon: ActivityIcon },
  { to: "/settings", key: "nav.settings", icon: SettingsIcon },
];

export function App() {
  const me = useMe();
  const qc = useQueryClient();
  useEffect(() => setUnauthorizedHandler(() => qc.setQueryData(["me"], null)), [qc]);
  if (me.isLoading) return <PageLoader />;
  if (!me.data) return <Login />;
  return (
    <Shell me={me.data}>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/users" element={<Users />} />
        <Route path="/users/:id" element={<Users />} />
        <Route path="/plans" element={<Plans />} />
        <Route path="/domains" element={<Domains />} />
        <Route path="/payments" element={<Payments />} />
        <Route path="/gates" element={<Gates />} />
        <Route path="/activity" element={<ActivityPage />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  );
}

function Shell({ me, children }: { me: Me; children: React.ReactNode }) {
  const { t, lang, setLang } = useI18n();
  const { dark, toggle } = useTheme();
  const [open, setOpen] = useState(false);
  const [palette, setPalette] = useState(false);
  const loc = useLocation();
  const qc = useQueryClient();
  useEffect(() => setOpen(false), [loc.pathname]);
  useEffect(() => {
    document.title = `${me.brand} · ${t("nav.console")}`;
  }, [me.brand, t]);
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPalette(true);
      }
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, []);

  const logout = async () => {
    await api("/logout", { method: "POST" }).catch(() => {});
    qc.setQueryData(["me"], null);
  };

  const sidebar = (
    <div className="flex flex-col h-full">
      <div className="h-16 px-5 flex items-center gap-3">
        <LogoMark size={34} />
        <div className="leading-tight min-w-0">
          <div className="font-extrabold truncate">{me.brand}</div>
          <div className="text-[11px] text-muted">{t("nav.console")}</div>
        </div>
      </div>
      <button onClick={() => setPalette(true)} className="mx-3 mb-3 h-10 px-3 rounded-xl bg-surface-2 border border-line text-sm text-faint flex items-center gap-2 hover:border-line-strong transition-colors">
        <Search className="size-4" />
        <span className="flex-1 text-start truncate">{t("nav.search")}</span>
        <kbd className="hidden lg:inline text-[10px] font-sans px-1.5 py-0.5 rounded-md border border-line bg-surface ltr">{t("nav.search.hint")}</kbd>
      </button>
      <nav className="px-3 space-y-0.5 flex-1 overflow-y-auto">
        {nav.map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            end={n.to === "/"}
            className={({ isActive }) =>
              cn(
                "relative flex items-center gap-3 h-10 px-3 rounded-xl text-sm font-semibold transition-all",
                isActive ? "bg-primary-soft text-primary" : "text-muted hover:text-fg hover:bg-surface-2",
              )
            }
          >
            {({ isActive }) => (
              <>
                {isActive && <span className="absolute start-0 top-2 bottom-2 w-1 rounded-full gradient-bg -ms-3" />}
                <n.icon className="size-[18px]" />
                <span className="flex-1">{t(n.key)}</span>
                {n.badge === "payments" && me.pending_payments > 0 && (
                  <span className="min-w-5 h-5 px-1.5 rounded-full bg-danger text-white text-[11px] font-bold grid place-items-center num">
                    {me.pending_payments.toLocaleString(lang === "fa" ? "fa-IR" : "en-US")}
                  </span>
                )}
              </>
            )}
          </NavLink>
        ))}
      </nav>
      <div className="p-3 border-t border-line">
        <div className="flex items-center gap-3 p-2 rounded-xl">
          <Avatar name={me.admin.username} size={36} />
          <div className="flex-1 min-w-0">
            <div className="text-sm font-bold truncate ltr text-start">{me.admin.username}</div>
            <div className="text-[11px] text-faint ltr text-start">{me.version}</div>
          </div>
          <IconButton label={t("nav.logout")} onClick={logout}>
            <LogOut className="size-[18px]" />
          </IconButton>
        </div>
      </div>
    </div>
  );

  return (
    <div className="min-h-dvh lg:ps-[264px]">
      <aside className="hidden lg:block fixed inset-y-0 start-0 w-[264px] border-e border-line bg-surface/70 backdrop-blur-xl z-30">{sidebar}</aside>
      {open && (
        <div className="lg:hidden fixed inset-0 z-50">
          <div className="absolute inset-0 bg-black/40 backdrop-blur-sm animate-in" onClick={() => setOpen(false)} />
          <aside className="absolute inset-y-0 start-0 w-[280px] bg-surface border-e border-line shadow-pop animate-in">{sidebar}</aside>
        </div>
      )}
      <header className="sticky top-0 z-20 glass border-x-0 border-t-0 h-16 flex items-center gap-2 px-4 lg:px-8">
        <IconButton label="menu" className="lg:hidden" onClick={() => setOpen(true)}>
          <Menu className="size-5" />
        </IconButton>
        <div className="lg:hidden flex items-center gap-2 me-auto">
          <LogoMark size={28} />
          <span className="font-extrabold">{me.brand}</span>
        </div>
        <div className="hidden lg:block me-auto" />
        <IconButton label={t("nav.search")} className="lg:hidden" onClick={() => setPalette(true)}>
          <Search className="size-[18px]" />
        </IconButton>
        <IconButton label={lang === "fa" ? "English" : "فارسی"} onClick={() => setLang(lang === "fa" ? "en" : "fa")}>
          <Languages className="size-[18px]" />
        </IconButton>
        <IconButton label="theme" onClick={toggle}>
          {dark ? <Sun className="size-[18px]" /> : <Moon className="size-[18px]" />}
        </IconButton>
      </header>
      <main className="px-4 lg:px-8 py-6 max-w-[1400px] mx-auto">{children}</main>
      {palette && <Palette onClose={() => setPalette(false)} />}
    </div>
  );
}

/** Ctrl-K: jump to a customer or a page by typing. */
function Palette({ onClose }: { onClose: () => void }) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  useEffect(() => input.current?.focus(), []);
  const users = useQuery({
    queryKey: ["palette", q],
    queryFn: () => api<{ users: User[] }>(`/users?q=${encodeURIComponent(q)}&limit=6`),
    enabled: q.trim().length > 0,
  });
  const items = useMemo(() => {
    const pages = nav.filter((n) => !q || t(n.key).toLowerCase().includes(q.toLowerCase())).map((n) => ({ kind: "page" as const, label: t(n.key), to: n.to, icon: n.icon }));
    const us = (users.data?.users || []).map((u) => ({ kind: "user" as const, label: u.display_name || u.username, sub: u.username, to: `/users/${u.id}`, icon: UsersIcon }));
    return [...us, ...pages];
  }, [q, users.data, t]);
  const go = (to: string) => {
    navigate(to);
    onClose();
  };
  return (
    <div className="fixed inset-0 z-[70] flex items-start justify-center pt-[12vh] px-4" onKeyDown={(e) => {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowDown") { e.preventDefault(); setSel((s) => Math.min(items.length - 1, s + 1)); }
      if (e.key === "ArrowUp") { e.preventDefault(); setSel((s) => Math.max(0, s - 1)); }
      if (e.key === "Enter" && items[sel]) go(items[sel].to);
    }}>
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm animate-in" onClick={onClose} />
      <div className="relative w-full max-w-xl card shadow-pop overflow-hidden animate-up">
        <div className="flex items-center gap-3 px-4 h-14 border-b border-line">
          <Search className="size-5 text-faint" />
          <input ref={input} value={q} onChange={(e) => { setQ(e.target.value); setSel(0); }} placeholder={t("nav.search")} className="flex-1 bg-transparent outline-none text-[15px]" />
          <IconButton label={t("ui.close")} onClick={onClose}><X className="size-4" /></IconButton>
        </div>
        <div className="max-h-[50vh] overflow-y-auto p-2">
          {items.length === 0 && <div className="text-sm text-muted text-center py-8">{t("nav.search.empty")}</div>}
          {items.map((it, i) => (
            <button key={it.kind + it.to} onMouseEnter={() => setSel(i)} onClick={() => go(it.to)}
              className={cn("w-full flex items-center gap-3 px-3 h-12 rounded-xl text-start", sel === i ? "bg-primary-soft" : "")}>
              {it.kind === "user" ? <Avatar name={it.sub || it.label} size={28} /> : <span className="size-7 rounded-lg bg-surface-2 grid place-items-center"><it.icon className="size-4 text-muted" /></span>}
              <span className="flex-1 min-w-0">
                <span className="block text-sm font-semibold truncate">{it.label}</span>
                {"sub" in it && it.sub && <span className="block text-xs text-muted ltr text-start">{it.sub}</span>}
              </span>
              {sel === i && <CornerDownLeft className="size-4 text-primary" />}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
