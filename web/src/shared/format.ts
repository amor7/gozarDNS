import type { Lang } from "./i18n";

const locale = (l: Lang) => (l === "fa" ? "fa-IR" : "en-US");

export function num(n: number, lang: Lang, digits = 0) {
  return new Intl.NumberFormat(locale(lang), { maximumFractionDigits: digits, minimumFractionDigits: 0 }).format(n);
}

const units = {
  fa: ["بایت", "کیلوبایت", "مگابایت", "گیگابایت", "ترابایت"],
  en: ["B", "KB", "MB", "GB", "TB"],
};

/** 33712345678 -> "31.4 GB" / "۳۱٫۴ گیگابایت" */
export function bytes(n: number, lang: Lang, digits?: number) {
  if (!n || n < 0) n = 0;
  let i = 0;
  let v = n;
  while (v >= 1024 && i < 4) {
    v /= 1024;
    i++;
  }
  const d = digits ?? (v >= 100 || i === 0 ? 0 : 1);
  return `${num(v, lang, d)} ${units[lang][i]}`;
}

/** Split form for big numbers in cards: value and unit separately. */
export function bytesParts(n: number, lang: Lang): [string, string] {
  const s = bytes(n, lang);
  const i = s.lastIndexOf(" ");
  return [s.slice(0, i), s.slice(i + 1)];
}

export const GB = 1024 ** 3;

export function speed(kbps: number, lang: Lang) {
  if (!kbps) return lang === "fa" ? "بدون محدودیت" : "Unlimited";
  if (kbps >= 1000) return `${num(kbps / 1000, lang, 1)} ${lang === "fa" ? "مگابیت" : "Mbps"}`;
  return `${num(kbps, lang)} ${lang === "fa" ? "کیلوبیت" : "kbps"}`;
}

export function rate(bps: number, lang: Lang) {
  return `${bytes(bps, lang)}/${lang === "fa" ? "ث" : "s"}`;
}

/** A date in the reader's calendar: Solar Hijri for Persian. */
export function date(ts: number | null | undefined, lang: Lang, withTime = false) {
  if (!ts) return "—";
  const opts: Intl.DateTimeFormatOptions = { year: "numeric", month: "long", day: "numeric" };
  if (withTime) Object.assign(opts, { hour: "2-digit", minute: "2-digit" });
  return new Intl.DateTimeFormat(lang === "fa" ? "fa-IR-u-ca-persian" : "en-GB", opts).format(new Date(ts * 1000));
}

export function shortDate(day: string, lang: Lang) {
  const d = new Date(day + "T12:00:00");
  return new Intl.DateTimeFormat(lang === "fa" ? "fa-IR-u-ca-persian" : "en-GB", { month: "short", day: "numeric" }).format(d);
}

export function time(ts: number, lang: Lang) {
  return new Intl.DateTimeFormat(lang === "fa" ? "fa-IR" : "en-GB", { hour: "2-digit", minute: "2-digit" }).format(new Date(ts * 1000));
}

/** "3 days ago", "in 5 hours" */
export function relative(ts: number | null | undefined, lang: Lang) {
  if (!ts) return "—";
  const diff = ts - Date.now() / 1000;
  const rtf = new Intl.RelativeTimeFormat(locale(lang), { numeric: "auto" });
  const abs = Math.abs(diff);
  if (abs < 60) return lang === "fa" ? "همین حالا" : "just now";
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  if (abs < 86400 * 45) return rtf.format(Math.round(diff / 86400), "day");
  return rtf.format(Math.round(diff / (86400 * 30)), "month");
}

export function daysLeft(ts: number | null | undefined) {
  if (!ts) return null;
  return Math.max(0, Math.ceil((ts - Date.now() / 1000) / 86400));
}

export function money(n: number, lang: Lang, currency: string) {
  return `${num(n, lang)} ${currency}`;
}

/** Persian/Arabic digits typed by the user, back to ASCII. */
export function latinDigits(s: string) {
  return s.replace(/[۰-۹]/g, (d) => String("۰۱۲۳۴۵۶۷۸۹".indexOf(d))).replace(/[٠-٩]/g, (d) => String("٠١٢٣٤٥٦٧٨٩".indexOf(d)));
}

export function uptime(sec: number, lang: Lang) {
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  if (lang === "fa") return d > 0 ? `${num(d, lang)} روز و ${num(h, lang)} ساعت` : `${num(h, lang)} ساعت`;
  return d > 0 ? `${d}d ${h}h` : `${h}h`;
}
