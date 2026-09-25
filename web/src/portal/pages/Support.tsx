import { ChevronLeft, ChevronRight, Clock, Headset, Mail, MessageCircle, MessagesSquare, Phone, Send, type LucideIcon } from "lucide-react";
import { useI18n } from "@shared/i18n";
import { Card, Empty, Lines } from "@shared/ui";
import { useInfo } from "../data";
import type { Contact } from "../types";

type Method = { key: keyof Contact; icon: LucideIcon; color: string; href: (v: string) => string; show: (v: string) => string };

// "0912..." -> "98912..." for wa.me, which wants the international form.
const intl = (v: string) => {
  const d = v.replace(/\D/g, "");
  if (d.startsWith("0")) return "98" + d.slice(1);
  return d;
};

const methods: Method[] = [
  { key: "telegram", icon: Send, color: "#229ED9", href: (v) => `https://t.me/${v}`, show: (v) => "@" + v },
  { key: "whatsapp", icon: MessageCircle, color: "#25D366", href: (v) => `https://wa.me/${intl(v)}`, show: (v) => v },
  { key: "bale", icon: MessagesSquare, color: "#00A78E", href: (v) => `https://ble.ir/${v}`, show: (v) => "@" + v },
  { key: "phone", icon: Phone, color: "var(--primary)", href: (v) => `tel:${v}`, show: (v) => v },
  { key: "email", icon: Mail, color: "var(--info)", href: (v) => `mailto:${v}`, show: (v) => v },
];

/** Whether the operator gave customers any way to reach them. */
export function hasSupport(c?: Contact, note?: string) {
  return !!note?.trim() || (!!c && methods.some((m) => c[m.key]));
}

export function Support() {
  const { t, dir } = useI18n();
  const info = useInfo().data;
  const c = info?.contact;
  const Arrow = dir === "rtl" ? ChevronLeft : ChevronRight;
  const list = methods.filter((m) => c?.[m.key]);

  return (
    <div className="space-y-4 animate-in max-w-2xl">
      <div>
        <h1 className="text-2xl font-extrabold">{t("sup.title")}</h1>
        <p className="text-muted mt-1">{t("sup.sub")}</p>
      </div>

      {!hasSupport(c, info?.support) ? (
        <Card><Empty icon={<Headset className="size-6" />} title={t("sup.none")} /></Card>
      ) : (
        <>
          {list.length > 0 && (
            <div className="grid sm:grid-cols-2 gap-3">
              {list.map((m) => {
                const v = c![m.key];
                return (
                  <a key={m.key} href={m.href(v)} target={m.key === "phone" || m.key === "email" ? undefined : "_blank"} rel="noreferrer"
                    className="card p-4 flex items-center gap-3.5 hover:-translate-y-0.5 transition-transform">
                    <span className="size-12 rounded-2xl grid place-items-center text-white shrink-0" style={{ background: m.color }}>
                      <m.icon className="size-6" />
                    </span>
                    <span className="flex-1 min-w-0">
                      <span className="block font-bold">{t("sup." + m.key)}</span>
                      <span className="block text-sm text-muted ltr text-start num truncate">{m.show(v)}</span>
                    </span>
                    <Arrow className="size-5 text-faint" />
                  </a>
                );
              })}
            </div>
          )}
          {c?.hours && (
            <Card className="p-4 flex items-center gap-3.5">
              <span className="size-12 rounded-2xl grid place-items-center bg-warning-soft text-warning shrink-0"><Clock className="size-6" /></span>
              <span>
                <span className="block font-bold">{t("sup.hours")}</span>
                <span className="block text-sm text-muted">{c.hours}</span>
              </span>
            </Card>
          )}
          {info?.support?.trim() && (
            <Card className="p-5">
              <Lines text={info.support} className="text-sm text-muted leading-relaxed" />
            </Card>
          )}
        </>
      )}
    </div>
  );
}
