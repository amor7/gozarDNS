import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { CircleAlert, CircleCheck, Info, X } from "lucide-react";
import { Button, cn } from "./ui";
import { useI18n } from "./i18n";

function useEscape(open: boolean, onClose: () => void) {
  useEffect(() => {
    if (!open) return;
    const on = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", on);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", on);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);
}

/** Centered dialog; becomes a bottom sheet on small screens. */
export function Modal({ open, onClose, title, sub, children, footer, size = "md" }: {
  open: boolean; onClose: () => void; title?: ReactNode; sub?: ReactNode; children: ReactNode; footer?: ReactNode; size?: "sm" | "md" | "lg" | "xl";
}) {
  useEscape(open, onClose);
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (open) panel.current?.querySelector<HTMLElement>("input, textarea, select")?.focus();
  }, [open]);
  if (!open) return null;
  const widths = { sm: "sm:max-w-sm", md: "sm:max-w-lg", lg: "sm:max-w-2xl", xl: "sm:max-w-4xl" };
  return createPortal(
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center sm:p-6">
      <div className="absolute inset-0 bg-black/50 backdrop-blur-sm animate-in" onClick={onClose} />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        className={cn(
          "relative w-full max-h-[92dvh] flex flex-col bg-surface border border-line shadow-pop animate-up",
          "rounded-t-3xl sm:rounded-3xl",
          widths[size],
        )}
      >
        <div className="sm:hidden mx-auto mt-2.5 h-1.5 w-10 rounded-full bg-line-strong" />
        {title && (
          <div className="flex items-start justify-between gap-4 px-6 pt-5 pb-2">
            <div>
              <h2 className="text-lg font-extrabold">{title}</h2>
              {sub && <p className="text-sm text-muted mt-1">{sub}</p>}
            </div>
            <button onClick={onClose} className="size-9 -me-2 rounded-xl grid place-items-center text-muted hover:text-fg hover:bg-surface-2" aria-label="close">
              <X className="size-5" />
            </button>
          </div>
        )}
        <div className="px-6 py-4 overflow-y-auto">{children}</div>
        {footer && <div className="px-6 py-4 border-t border-line flex flex-wrap gap-2 justify-end">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}

/** Side panel for details. Slides from the reading end. */
export function Drawer({ open, onClose, children, width = "max-w-xl" }: { open: boolean; onClose: () => void; children: ReactNode; width?: string }) {
  useEscape(open, onClose);
  const { dir } = useI18n();
  if (!open) return null;
  return createPortal(
    <div className="fixed inset-0 z-50">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-[2px] animate-in" onClick={onClose} />
      <div
        className={cn("absolute inset-y-0 end-0 w-full bg-bg border-s border-line shadow-pop overflow-y-auto", width)}
        style={{ animation: `${dir === "rtl" ? "gz-in-left" : "gz-in-right"} .28s cubic-bezier(.2,.8,.2,1)` }}
      >
        {children}
      </div>
      <style>{`@keyframes gz-in-right{from{transform:translateX(40px);opacity:0}to{transform:none;opacity:1}}@keyframes gz-in-left{from{transform:translateX(-40px);opacity:0}to{transform:none;opacity:1}}`}</style>
    </div>,
    document.body,
  );
}

// ---------------------------------------------------------------- toasts

type Toast = { id: number; kind: "success" | "error" | "info"; text: string };
type ToastCtx = { push: (kind: Toast["kind"], text: string) => void };
const ToastContext = createContext<ToastCtx | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<Toast[]>([]);
  const push = useCallback((kind: Toast["kind"], text: string) => {
    const id = Date.now() + Math.random();
    setItems((x) => [...x.slice(-3), { id, kind, text }]);
    setTimeout(() => setItems((x) => x.filter((t) => t.id !== id)), kind === "error" ? 6000 : 3500);
  }, []);
  return (
    <ToastContext.Provider value={{ push }}>
      {children}
      {createPortal(
        <div className="fixed z-[60] bottom-4 inset-x-4 sm:inset-x-auto sm:end-6 sm:bottom-6 flex flex-col gap-2 sm:w-96 pointer-events-none">
          {items.map((t) => (
            <div key={t.id} className="pointer-events-auto glass rounded-2xl shadow-pop px-4 py-3 flex items-start gap-3 animate-up">
              {t.kind === "success" ? (
                <CircleCheck className="size-5 text-success shrink-0 mt-0.5" />
              ) : t.kind === "error" ? (
                <CircleAlert className="size-5 text-danger shrink-0 mt-0.5" />
              ) : (
                <Info className="size-5 text-info shrink-0 mt-0.5" />
              )}
              <div className="text-sm font-medium leading-relaxed">{t.text}</div>
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

export function useToast() {
  const c = useContext(ToastContext);
  if (!c) throw new Error("useToast outside provider");
  const { t } = useI18n();
  return {
    success: (text: string) => c.push("success", text),
    info: (text: string) => c.push("info", text),
    error: (e: unknown) => {
      const code = (e as { code?: string })?.code || "internal";
      c.push("error", t("err." + code));
    },
  };
}

// ---------------------------------------------------------------- confirm

type ConfirmOpts = { title: string; body?: ReactNode; confirm?: string; danger?: boolean };
const ConfirmContext = createContext<((o: ConfirmOpts) => Promise<boolean>) | null>(null);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<(ConfirmOpts & { resolve: (v: boolean) => void }) | null>(null);
  const { t } = useI18n();
  const ask = useCallback((o: ConfirmOpts) => new Promise<boolean>((resolve) => setState({ ...o, resolve })), []);
  const close = (v: boolean) => {
    state?.resolve(v);
    setState(null);
  };
  return (
    <ConfirmContext.Provider value={ask}>
      {children}
      <Modal
        open={!!state}
        onClose={() => close(false)}
        title={state?.title}
        size="sm"
        footer={
          <>
            <Button variant="secondary" onClick={() => close(false)}>
              {t("ui.cancel")}
            </Button>
            <Button variant={state?.danger ? "danger" : "primary"} onClick={() => close(true)}>
              {state?.confirm || t("ui.confirm")}
            </Button>
          </>
        }
      >
        <div className="text-sm text-muted leading-relaxed">{state?.body}</div>
      </Modal>
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  const c = useContext(ConfirmContext);
  if (!c) throw new Error("useConfirm outside provider");
  return c;
}
