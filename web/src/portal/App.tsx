import { useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation } from "react-router";
import { CloudOff, Compass, CreditCard, Headset, House, Languages, Megaphone, Moon, RefreshCw, Sun, UserRound, X, type LucideIcon } from "lucide-react";
import { useI18n } from "@shared/i18n";
import { useTheme } from "@shared/theme";
import { Button, cn, IconButton, Lines, LogoMark, PageLoader } from "@shared/ui";
import { useInfo, useMe } from "./data";
import { Welcome } from "./pages/Welcome";
import { Login, Signup } from "./pages/Auth";
import { Home } from "./pages/Home";
import { Setup } from "./pages/Setup";
import { Plans } from "./pages/Plans";
import { Account } from "./pages/Account";
import { hasSupport, Support } from "./pages/Support";

const allTabs: { to: string; key: string; icon: LucideIcon }[] = [
  { to: "/", key: "nav.home", icon: House },
  { to: "/setup", key: "nav.setup", icon: Compass },
  { to: "/plans", key: "nav.plans", icon: CreditCard },
  { to: "/support", key: "nav.support", icon: Headset },
  { to: "/account", key: "nav.account", icon: UserRound },
];

export function App() {
  const me = useMe();
  const info = useInfo();
  const loc = useLocation();

  if (me.isLoading || info.isLoading) return <PageLoader />;
  if (info.isError || me.isError) return <Shell brand={info.data?.brand}><Unreachable onRetry={() => { info.refetch(); me.refetch(); }} /></Shell>;
  const signedIn = !!me.data;

  if (!signedIn) {
    return (
      <Shell brand={info.data?.brand}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/signup" element={<Signup />} />
          <Route path="/" element={<Welcome />} />
          <Route path="*" element={<Navigate to={loc.pathname === "/" ? "/" : "/login"} replace />} />
        </Routes>
      </Shell>
    );
  }

  return (
    <Shell brand={info.data?.brand} nav>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/setup" element={<Setup />} />
        <Route path="/plans" element={<Plans />} />
        <Route path="/account" element={<Account />} />
        <Route path="/support" element={<Support />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  );
}

/** The gate is up but cannot reach the hub, which holds every account. */
function Unreachable({ onRetry }: { onRetry: () => void }) {
  const { t } = useI18n();
  return (
    <div className="max-w-md mx-auto pt-16 text-center animate-in">
      <div className="size-16 mx-auto rounded-2xl bg-warning-soft text-warning grid place-items-center">
        <CloudOff className="size-8" />
      </div>
      <h1 className="text-xl font-extrabold mt-5">{t("down.t")}</h1>
      <p className="text-muted mt-2 leading-relaxed">{t("down.s")}</p>
      <Button className="mt-6" onClick={onRetry} icon={<RefreshCw className="size-4" />}>{t("ui.retry")}</Button>
    </div>
  );
}

/** The operator's announcement, above every page. Closing it hides this
 *  text only; a new announcement shows again. */
function Notice() {
  const info = useInfo().data;
  const text = info?.notice?.trim() || "";
  const key = "gz-notice-closed";
  const [closed, setClosed] = useState(() => {
    try {
      return localStorage.getItem(key) || "";
    } catch {
      return "";
    }
  });
  if (!text || closed === text) return null;
  return (
    <div className="max-w-5xl mx-auto px-4 pt-4 animate-in">
      <div className="rounded-2xl gradient-bg text-white p-4 flex gap-3 items-start shadow-[0_12px_32px_-16px_var(--primary)]">
        <Megaphone className="size-5 shrink-0 mt-0.5" />
        <Lines text={text} className="text-sm leading-relaxed flex-1 font-medium" />
        <button
          onClick={() => {
            setClosed(text);
            try {
              localStorage.setItem(key, text);
            } catch {
              /* no storage: closed for this visit */
            }
          }}
          className="size-7 -m-1 rounded-lg grid place-items-center text-white/80 hover:text-white hover:bg-white/15 shrink-0"
          aria-label="close"
        >
          <X className="size-4" />
        </button>
      </div>
    </div>
  );
}

function Shell({ brand, nav, children }: { brand?: string; nav?: boolean; children: React.ReactNode }) {
  const { t, lang, setLang } = useI18n();
  const { dark, toggle } = useTheme();
  const info = useInfo().data;
  // No contact details set: no support tab to find an empty page behind.
  const tabs = allTabs.filter((x) => x.to !== "/support" || hasSupport(info?.contact, info?.support));
  return (
    <div className={cn("min-h-dvh", nav && "pb-24 sm:pb-10")}>
      <header className="sticky top-0 z-30">
        <div className="glass border-x-0 border-t-0">
          <div className="max-w-5xl mx-auto px-4 h-16 flex items-center gap-3">
            <NavLink to="/" className="flex items-center gap-2.5 me-auto">
              <LogoMark size={34} />
              <span className="font-extrabold text-[17px] tracking-tight">{brand || "Gozar"}</span>
            </NavLink>
            {nav && (
              <nav className="hidden sm:flex items-center gap-1 me-2">
                {tabs.map((tab) => (
                  <NavLink
                    key={tab.to}
                    to={tab.to}
                    end
                    className={({ isActive }) =>
                      cn("h-9 px-3.5 rounded-xl text-sm font-semibold flex items-center gap-2 transition-colors", isActive ? "bg-primary-soft text-primary" : "text-muted hover:text-fg hover:bg-surface-2")
                    }
                  >
                    <tab.icon className="size-4" />
                    {t(tab.key)}
                  </NavLink>
                ))}
              </nav>
            )}
            <IconButton label={lang === "fa" ? "English" : "فارسی"} onClick={() => setLang(lang === "fa" ? "en" : "fa")}>
              <Languages className="size-[18px]" />
            </IconButton>
            <IconButton label="theme" onClick={toggle}>
              {dark ? <Sun className="size-[18px]" /> : <Moon className="size-[18px]" />}
            </IconButton>
          </div>
        </div>
      </header>

      <Notice />
      <main className="max-w-5xl mx-auto px-4 pt-6">{children}</main>

      {nav && (
        <nav className="sm:hidden fixed bottom-0 inset-x-0 z-30 glass border-x-0 border-b-0 pb-[env(safe-area-inset-bottom)]">
          <div className="flex h-16 [&>a]:flex-1">
            {tabs.map((tab) => (
              <NavLink
                key={tab.to}
                to={tab.to}
                end
                className={({ isActive }) => cn("flex flex-col items-center justify-center gap-1 text-[11px] font-semibold transition-colors", isActive ? "text-primary" : "text-faint")}
              >
                {({ isActive }) => (
                  <>
                    <span className={cn("h-7 w-12 rounded-full grid place-items-center transition-all", isActive && "bg-primary-soft")}>
                      <tab.icon className="size-[19px]" />
                    </span>
                    {t(tab.key)}
                  </>
                )}
              </NavLink>
            ))}
          </div>
        </nav>
      )}
    </div>
  );
}
