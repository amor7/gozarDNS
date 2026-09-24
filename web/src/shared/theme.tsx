import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

export type ThemePref = "system" | "light" | "dark";

type Ctx = { pref: ThemePref; dark: boolean; setPref: (p: ThemePref) => void; toggle: () => void };
const ThemeCtx = createContext<Ctx | null>(null);

function stored(): ThemePref {
  try {
    const v = localStorage.getItem("gz-theme");
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    /* no storage */
  }
  return "system";
}

const media = () => window.matchMedia?.("(prefers-color-scheme: dark)");

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [pref, setPrefState] = useState<ThemePref>(stored);
  const [systemDark, setSystemDark] = useState(() => media()?.matches ?? true);

  useEffect(() => {
    const m = media();
    if (!m) return;
    const on = (e: MediaQueryListEvent) => setSystemDark(e.matches);
    m.addEventListener("change", on);
    return () => m.removeEventListener("change", on);
  }, []);

  const dark = pref === "dark" || (pref === "system" && systemDark);
  useEffect(() => {
    document.documentElement.classList.toggle("dark", dark);
    document.querySelector('meta[name="theme-color"]')?.setAttribute("content", dark ? "#0a0a12" : "#f6f6fb");
  }, [dark]);

  const setPref = (p: ThemePref) => {
    setPrefState(p);
    try {
      localStorage.setItem("gz-theme", p);
    } catch {
      /* ignore */
    }
  };
  return (
    <ThemeCtx.Provider value={{ pref, dark, setPref, toggle: () => setPref(dark ? "light" : "dark") }}>
      {children}
    </ThemeCtx.Provider>
  );
}

export function useTheme() {
  const c = useContext(ThemeCtx);
  if (!c) throw new Error("useTheme outside provider");
  return c;
}
