import { forwardRef, useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";
import { Check, Copy, Eye, EyeOff, Loader2 } from "lucide-react";
import { useI18n } from "./i18n";

export function cn(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(" ");
}

// ---------------------------------------------------------------- brand

/** The mark: two arcs forming a gateway, a dot passing through it. */
export function LogoMark({ size = 32, className }: { size?: number; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 40 40" className={className} aria-hidden>
      <defs>
        <linearGradient id="gz-g" x1="0" y1="0" x2="40" y2="40" gradientUnits="userSpaceOnUse">
          <stop stopColor="var(--primary)" />
          <stop offset="1" stopColor="var(--primary-2)" />
        </linearGradient>
      </defs>
      <rect x="1" y="1" width="38" height="38" rx="12" fill="url(#gz-g)" />
      <path d="M12 28V19a8 8 0 0 1 16 0v9" fill="none" stroke="white" strokeWidth="3.2" strokeLinecap="round" />
      <path d="M17 28v-8.5a3 3 0 0 1 6 0V28" fill="none" stroke="white" strokeOpacity=".55" strokeWidth="2.6" strokeLinecap="round" />
      <circle cx="20" cy="11.5" r="2.4" fill="white" />
    </svg>
  );
}

export function Logo({ name = "Gozar", sub }: { name?: string; sub?: string }) {
  return (
    <div className="flex items-center gap-2.5">
      <LogoMark />
      <div className="leading-tight">
        <div className="font-extrabold tracking-tight text-[17px]">{name}</div>
        {sub && <div className="text-[11px] text-muted">{sub}</div>}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------- buttons

type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger" | "soft" | "success";
  size?: "sm" | "md" | "lg";
  loading?: boolean;
  icon?: ReactNode;
  block?: boolean;
};

export const Button = forwardRef<HTMLButtonElement, BtnProps>(function Button(
  { variant = "primary", size = "md", loading, icon, block, className, children, disabled, ...rest },
  ref,
) {
  const base =
    "inline-flex items-center justify-center gap-2 font-semibold rounded-xl transition-all duration-150 select-none disabled:opacity-50 disabled:pointer-events-none active:scale-[0.98] whitespace-nowrap";
  const sizes = { sm: "h-8 px-3 text-[13px]", md: "h-10 px-4 text-sm", lg: "h-12 px-6 text-[15px]" };
  const variants = {
    primary: "gradient-bg text-on-primary shadow-[0_8px_24px_-10px_var(--primary)] hover:brightness-110 hover:shadow-[0_10px_28px_-8px_var(--primary)]",
    secondary: "bg-surface-2 text-fg border border-line hover:bg-surface-3 hover:border-line-strong",
    ghost: "text-muted hover:text-fg hover:bg-surface-2",
    danger: "bg-danger text-white hover:brightness-110",
    soft: "bg-primary-soft text-primary hover:brightness-110",
    success: "bg-success text-white hover:brightness-110",
  };
  return (
    <button
      ref={ref}
      className={cn(base, sizes[size], variants[variant], block && "w-full", className)}
      disabled={disabled || loading}
      {...rest}
    >
      {loading ? <Loader2 className="size-4 animate-spin" /> : icon}
      {children}
    </button>
  );
});

export function IconButton({ label, className, children, ...rest }: ButtonHTMLAttributes<HTMLButtonElement> & { label: string }) {
  return (
    <button
      aria-label={label}
      title={label}
      className={cn("inline-grid place-items-center size-9 rounded-xl text-muted hover:text-fg hover:bg-surface-2 transition-colors", className)}
      {...rest}
    >
      {children}
    </button>
  );
}

// ---------------------------------------------------------------- surfaces

export function Card({ className, children, ...rest }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn("card", className)} {...rest}>
      {children}
    </div>
  );
}

export function CardHeader({ title, sub, action, icon }: { title: ReactNode; sub?: ReactNode; action?: ReactNode; icon?: ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-3 px-5 pt-5 pb-3">
      <div className="flex items-center gap-3 min-w-0">
        {icon && <div className="size-9 shrink-0 rounded-xl bg-primary-soft text-primary grid place-items-center">{icon}</div>}
        <div className="min-w-0">
          <h3 className="font-bold text-[15px] truncate">{title}</h3>
          {sub && <p className="text-[13px] text-muted mt-0.5">{sub}</p>}
        </div>
      </div>
      {action}
    </div>
  );
}

export function Stat({ label, value, unit, icon, tone = "primary", foot }: {
  label: string; value: ReactNode; unit?: string; icon: ReactNode; tone?: "primary" | "success" | "warning" | "danger" | "info"; foot?: ReactNode;
}) {
  const tones = {
    primary: "bg-primary-soft text-primary",
    success: "bg-success-soft text-success",
    warning: "bg-warning-soft text-warning",
    danger: "bg-danger-soft text-danger",
    info: "bg-info-soft text-info",
  };
  return (
    <Card className="p-5 animate-in">
      <div className="flex items-center justify-between">
        <span className="text-[13px] text-muted font-medium">{label}</span>
        <span className={cn("size-9 rounded-xl grid place-items-center", tones[tone])}>{icon}</span>
      </div>
      <div className="mt-3 flex items-baseline gap-1.5">
        <span className="text-[28px] font-extrabold tracking-tight num leading-none">{value}</span>
        {unit && <span className="text-sm text-muted font-medium">{unit}</span>}
      </div>
      {foot && <div className="mt-2 text-xs text-muted">{foot}</div>}
    </Card>
  );
}

// ---------------------------------------------------------------- forms

export function Field({ label, hint, error, children, className }: { label?: ReactNode; hint?: ReactNode; error?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <label className={cn("block", className)}>
      {label && <span className="block text-[13px] font-semibold mb-1.5">{label}</span>}
      {children}
      {error ? <span className="block text-xs text-danger mt-1.5">{error}</span> : hint ? <span className="block text-xs text-muted mt-1.5">{hint}</span> : null}
    </label>
  );
}

const inputCls =
  "w-full h-11 rounded-xl bg-surface-2 border border-line px-3.5 text-[14px] placeholder:text-faint transition-colors hover:border-line-strong focus:border-primary focus:bg-surface focus:outline-none focus:ring-4 focus:ring-primary-soft";

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { ltr?: boolean }>(function Input({ className, ltr, ...rest }, ref) {
  return <input ref={ref} className={cn(inputCls, ltr && "ltr text-start", className)} {...rest} />;
});

export function PasswordInput(props: InputHTMLAttributes<HTMLInputElement>) {
  const [show, setShow] = useState(false);
  const { t } = useI18n();
  return (
    <div className="relative">
      <Input {...props} type={show ? "text" : "password"} ltr className="pr-11" />
      <button type="button" onClick={() => setShow(!show)} className="absolute inset-y-0 right-0 px-3 text-faint hover:text-fg" aria-label={t(show ? "ui.hide" : "ui.show")}>
        {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}

export function Textarea({ className, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(inputCls, "h-auto min-h-24 py-2.5 leading-relaxed", className)} {...rest} />;
}

export function Select({ className, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select className={cn(inputCls, "appearance-none bg-[length:16px] bg-no-repeat pe-9", className)} style={{ backgroundImage: "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%239a97ad' stroke-width='2'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E\")", backgroundPosition: "var(--sel-pos, left 12px center)" }} {...rest}>
      {children}
    </select>
  );
}

export function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label?: ReactNode; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className="inline-flex items-center gap-2.5 disabled:opacity-50 text-start"
    >
      <span className={cn("relative inline-block h-6 w-11 shrink-0 rounded-full transition-colors", checked ? "gradient-bg" : "bg-surface-3 border border-line")}>
        <span
          className={cn(
            "absolute top-0.5 size-5 rounded-full bg-white shadow transition-all",
            checked ? "start-[calc(100%-1.375rem)]" : "start-0.5",
          )}
        />
      </span>
      {label && <span className="text-sm">{label}</span>}
    </button>
  );
}

// ---------------------------------------------------------------- display

type Tone = "neutral" | "primary" | "success" | "warning" | "danger" | "info";
const toneCls: Record<Tone, string> = {
  neutral: "bg-surface-2 text-muted border-line",
  primary: "bg-primary-soft text-primary border-transparent",
  success: "bg-success-soft text-success border-transparent",
  warning: "bg-warning-soft text-warning border-transparent",
  danger: "bg-danger-soft text-danger border-transparent",
  info: "bg-info-soft text-info border-transparent",
};

export function Badge({ tone = "neutral", dot, children, className }: { tone?: Tone; dot?: boolean; children: ReactNode; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full text-xs font-semibold border whitespace-nowrap", toneCls[tone], className)}>
      {dot && <span className="size-1.5 rounded-full bg-current" />}
      {children}
    </span>
  );
}

export function Progress({ value, max, className, tone }: { value: number; max: number; className?: string; tone?: "auto" | "primary" }) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  let bar = "gradient-bg";
  if (tone !== "primary") {
    if (pct >= 95) bar = "bg-danger";
    else if (pct >= 80) bar = "bg-warning";
  }
  return (
    <div className={cn("h-2 rounded-full bg-surface-3 overflow-hidden", className)}>
      <div className={cn("h-full rounded-full transition-[width] duration-700", bar)} style={{ width: `${max > 0 ? Math.max(pct, value > 0 ? 2 : 0) : 0}%` }} />
    </div>
  );
}

/** A donut gauge. */
export type RingTone = "primary" | "warning" | "danger";

export function Ring({ value, max, tone = "primary", size = 180, stroke = 14, children }: { value: number; max: number; tone?: RingTone; size?: number; stroke?: number; children?: ReactNode }) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const pct = max > 0 ? Math.min(1, Math.max(0, value / max)) : 0;
  const from = tone === "primary" ? "var(--primary)" : `var(--${tone})`;
  const to = tone === "primary" ? "var(--primary-2)" : `var(--${tone})`;
  const id = `ring-${size}-${stroke}-${tone}`;
  // Round caps reach stroke/2 past each end of the arc, which hid small gaps
  // (2 GB used of 150 still looked full). Draw the arc shorter by the caps,
  // start it half a cap late, and keep any gap at least a little visible.
  const minGap = stroke * 0.7;
  const arc = pct >= 1 ? c : Math.max(0, Math.min(pct * c - stroke, c - stroke - minGap));
  return (
    <div className="relative inline-grid place-items-center" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90">
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor={from} />
            <stop offset="1" stopColor={to} />
          </linearGradient>
        </defs>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={tone === "primary" ? "var(--surface-3)" : from} strokeOpacity={tone === "primary" ? 1 : 0.22} strokeWidth={stroke} />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={`url(#${id})`}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={`${arc} ${c}`}
          strokeDashoffset={pct >= 1 ? 0 : -stroke / 2}
          opacity={pct > 0 ? 1 : 0}
          style={{ transition: "stroke-dasharray 1s cubic-bezier(.2,.8,.2,1)" }}
        />
      </svg>
      <div className="absolute inset-0 grid place-items-center text-center">{children}</div>
    </div>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn("skeleton", className)} />;
}

export function Empty({ icon, title, sub, action }: { icon: ReactNode; title: ReactNode; sub?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center text-center py-14 px-6 animate-in">
      <div className="size-14 rounded-2xl bg-primary-soft text-primary grid place-items-center mb-4">{icon}</div>
      <div className="font-bold">{title}</div>
      {sub && <div className="text-sm text-muted mt-1 max-w-sm">{sub}</div>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

export function Segmented<T extends string>({ value, onChange, options, className }: { value: T; onChange: (v: T) => void; options: { value: T; label: ReactNode }[]; className?: string }) {
  return (
    <div className={cn("inline-flex p-1 rounded-xl bg-surface-2 border border-line gap-1", className)}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={cn(
            "h-8 px-3 rounded-lg text-[13px] font-semibold transition-all whitespace-nowrap",
            value === o.value ? "bg-surface text-fg shadow-card" : "text-muted hover:text-fg",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function useCopy() {
  const [copied, setCopied] = useState<string | null>(null);
  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      const ta = document.createElement("textarea");
      ta.value = text;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      ta.remove();
    }
    setCopied(text);
    setTimeout(() => setCopied(null), 1600);
  };
  return { copied, copy };
}

export function CopyButton({ text, label, className, size = "sm" }: { text: string; label?: string; className?: string; size?: "sm" | "md" }) {
  const { copied, copy } = useCopy();
  const { t } = useI18n();
  const done = copied === text;
  return (
    <Button type="button" variant={done ? "success" : "secondary"} size={size} onClick={() => copy(text)} className={className} icon={done ? <Check className="size-4" /> : <Copy className="size-4" />}>
      {label !== undefined ? (done ? t("ui.copied") : label) : done ? t("ui.copied") : t("ui.copy")}
    </Button>
  );
}

const avatarHues = [262, 190, 330, 150, 25, 210, 290, 170];
export function Avatar({ name, size = 36 }: { name: string; size?: number }) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  const hue = avatarHues[h % avatarHues.length];
  const initials = name.trim().slice(0, 2).toUpperCase();
  return (
    <span
      className="inline-grid place-items-center rounded-xl font-bold text-white shrink-0 ltr"
      style={{ width: size, height: size, fontSize: size * 0.38, background: `linear-gradient(135deg, hsl(${hue} 80% 62%), hsl(${(hue + 40) % 360} 75% 52%))` }}
    >
      {initials}
    </span>
  );
}

/** A small square in a service's brand colour, with its initial. */
export function ServiceDot({ name, color, size = 28 }: { name: string; color?: string; size?: number }) {
  return (
    <span
      className="inline-grid place-items-center rounded-lg font-bold text-white shrink-0 ltr"
      style={{ width: size, height: size, fontSize: size * 0.42, background: color || "var(--faint)" }}
      title={name}
    >
      {name.replace(/[^A-Za-z0-9؀-ۿ]/g, "").slice(0, 1).toUpperCase()}
    </span>
  );
}

/** Operator-written text, line by line. Each line picks its own direction so
 *  a card or phone number keeps its digit groups in order in Persian text. */
export function Lines({ text, className }: { text: string; className?: string }) {
  const { lang } = useI18n();
  return (
    <div className={className}>
      {text.split("\n").map((line, i) => (
        <div key={i} dir="auto" style={{ textAlign: lang === "fa" ? "right" : "left" }}>
          {line || " "}
        </div>
      ))}
    </div>
  );
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn("size-5 animate-spin text-primary", className)} />;
}

export function PageLoader() {
  return (
    <div className="grid place-items-center min-h-[60dvh]">
      <Spinner className="size-7" />
    </div>
  );
}
