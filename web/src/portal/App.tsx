import { NavLink, Navigate, Route, Routes, useLocation } from "react-router";
import { Compass, CreditCard, House, Languages, Moon, Sun, UserRound, type LucideIcon } from "lucide-react";
import { useI18n } from "@shared/i18n";
import { useTheme } from "@shared/theme";
import { cn, IconButton, LogoMark, PageLoader } from "@shared/ui";
import { useInfo, useMe } from "./data";
import { Welcome } from "./pages/Welcome";
import { Login, Signup } from "./pages/Auth";
import { Home } from "./pages/Home";
import { Setup } from "./pages/Setup";
import { Plans } from "./pages/Plans";
import { Account } from "./pages/Account";

const tabs: { to: string; key: string; icon: LucideIcon }[] = [
  { to: "/", key: "nav.home", icon: House },
  { to: "/setup", key: "nav.setup", icon: Compass },
  { to: "/plans", key: "nav.plans", icon: CreditCard },
  { to: "/account", key: "nav.account", icon: UserRound },
];

export function App() {
  const me = useMe();
  const info = useInfo();
  const loc = useLocation();

  if (me.isLoading || info.isLoading) return <PageLoader />;
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
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  );
}

function Shell({ brand, nav, children }: { brand?: string; nav?: boolean; children: React.ReactNode }) {
  const { t, lang, setLang } = useI18n();
  const { dark, toggle } = useTheme();
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

      <main className="max-w-5xl mx-auto px-4 pt-6">{children}</main>

      {nav && (
        <nav className="sm:hidden fixed bottom-0 inset-x-0 z-30 glass border-x-0 border-b-0 pb-[env(safe-area-inset-bottom)]">
          <div className="grid grid-cols-4 h-16">
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
