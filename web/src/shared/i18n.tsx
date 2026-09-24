import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Lang = "fa" | "en";
export type Dict = Record<string, string>;
export type Dicts = { fa: Dict; en: Dict };

type Ctx = {
  lang: Lang;
  dir: "rtl" | "ltr";
  setLang: (l: Lang) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
};

const I18n = createContext<Ctx | null>(null);

function stored(): Lang | null {
  try {
    const v = localStorage.getItem("gz-lang");
    return v === "fa" || v === "en" ? v : null;
  } catch {
    return null;
  }
}

function initial(): Lang {
  const s = stored();
  if (s) return s;
  // Persian unless the browser clearly prefers something else.
  const nav = (navigator.language || "fa").toLowerCase();
  return nav.startsWith("en") ? "en" : "fa";
}

export function I18nProvider({ dicts, children }: { dicts: Dicts; children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initial);
  const dir = lang === "fa" ? "rtl" : "ltr";

  useEffect(() => {
    document.documentElement.lang = lang;
    document.documentElement.dir = dir;
  }, [lang, dir]);

  const setLang = useCallback((l: Lang) => {
    setLangState(l);
    try {
      localStorage.setItem("gz-lang", l);
    } catch {
      /* private mode: the choice lasts for this visit */
    }
  }, []);

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      let s = dicts[lang][key] ?? dicts.en[key] ?? key;
      if (vars) {
        for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v));
      }
      return s;
    },
    [dicts, lang],
  );

  const value = useMemo(() => ({ lang, dir: dir as "rtl" | "ltr", setLang, t }), [lang, dir, setLang, t]);
  return <I18n.Provider value={value}>{children}</I18n.Provider>;
}

export function useI18n() {
  const c = useContext(I18n);
  if (!c) throw new Error("useI18n outside provider");
  return c;
}

/** Messages both apps show for API error codes. */
export const errorsFa: Dict = {
  "err.bad_request": "درخواست قابل خواندن نبود.",
  "err.unauthorized": "لطفاً دوباره وارد شوید.",
  "err.forbidden": "اجازهٔ این کار را ندارید.",
  "err.not_found": "پیدا نشد.",
  "err.internal": "مشکلی در سرور پیش آمد. کمی بعد دوباره امتحان کنید.",
  "err.network": "به سرور وصل نشد. اینترنت خود را بررسی کنید.",
  "err.hub_unreachable": "سرور اصلی موقتاً در دسترس نیست. چند دقیقهٔ دیگر امتحان کنید.",
  "err.invalid_credentials": "نام کاربری یا رمز عبور درست نیست.",
  "err.too_many_attempts": "تلاش‌ها زیاد شد. چند دقیقه صبر کنید.",
  "err.signup_closed": "ثبت‌نام فعلاً بسته است.",
  "err.username_invalid": "نام کاربری باید ۳ تا ۳۲ حرف انگلیسی، عدد، نقطه، خط تیره یا زیرخط باشد.",
  "err.username_taken": "این نام کاربری قبلاً گرفته شده.",
  "err.password_short": "رمز عبور کوتاه است.",
  "err.password_wrong": "رمز عبور فعلی درست نیست.",
  "err.ip_taken": "این آی‌پی به حساب دیگری ثبت شده است.",
  "err.ip_already_yours": "این آی‌پی از قبل به نام شما ثبت شده است.",
  "err.ip_invalid": "آی‌پی معتبر نیست.",
  "err.device_limit": "به سقف تعداد دستگاه رسیده‌اید.",
  "err.ipv6_unsupported": "اتصال شما IPv6 است. IPv6 را در مودم یا گوشی خاموش کنید و صفحه را دوباره باز کنید.",
  "err.receipt_missing": "تصویر رسید را انتخاب کنید.",
  "err.receipt_too_large": "حجم فایل بیشتر از ۵ مگابایت است.",
  "err.receipt_type": "فقط عکس (JPG، PNG، WEBP) یا PDF.",
  "err.receipts_pending": "چند رسید شما هنوز در انتظار بررسی است.",
  "err.plan_in_use": "اول مشتری‌های این پلن را به پلن دیگری ببرید.",
  "err.name_required": "نام را وارد کنید.",
  "err.domain_invalid": "این یک دامنهٔ معتبر نیست.",
  "err.totp_required": "کد برنامهٔ احراز هویت را وارد کنید.",
  "err.totp_invalid": "کد درست نیست. ساعت گوشی را بررسی کنید.",
  "err.totp_enabled": "ورود دومرحله‌ای از قبل روشن است.",
  "err.already_decided": "این رسید قبلاً بررسی شده است.",
  "err.last_admin": "آخرین مدیر را نمی‌شود حذف کرد.",
  "err.cannot_delete_self": "نمی‌توانید خودتان را حذف کنید.",
  "err.backup_missing": "فایل پشتیبان را انتخاب کنید.",
  "err.backup_invalid": "این فایل پشتیبان معتبر نیست.",
};

export const errorsEn: Dict = {
  "err.bad_request": "The request could not be read.",
  "err.unauthorized": "Please sign in again.",
  "err.forbidden": "You are not allowed to do that.",
  "err.not_found": "Not found.",
  "err.internal": "Something went wrong on the server. Try again in a moment.",
  "err.network": "Could not reach the server. Check your connection.",
  "err.hub_unreachable": "The main server is unreachable right now. Try again in a few minutes.",
  "err.invalid_credentials": "Wrong username or password.",
  "err.too_many_attempts": "Too many attempts. Wait a few minutes.",
  "err.signup_closed": "Sign-up is closed right now.",
  "err.username_invalid": "Usernames are 3-32 English letters, digits, dot, dash or underscore.",
  "err.username_taken": "That username is taken.",
  "err.password_short": "The password is too short.",
  "err.password_wrong": "The current password is not right.",
  "err.ip_taken": "This address is registered to another account.",
  "err.ip_already_yours": "This address is already registered to you.",
  "err.ip_invalid": "Not a valid IP address.",
  "err.device_limit": "You have reached your device limit.",
  "err.ipv6_unsupported": "You are connected over IPv6. Turn IPv6 off on your router or phone and reload.",
  "err.receipt_missing": "Choose a photo of the receipt.",
  "err.receipt_too_large": "The file is larger than 5 MB.",
  "err.receipt_type": "Photos (JPG, PNG, WEBP) or PDF only.",
  "err.receipts_pending": "You already have receipts waiting for review.",
  "err.plan_in_use": "Move this plan's customers to another plan first.",
  "err.name_required": "Enter a name.",
  "err.domain_invalid": "That is not a valid domain.",
  "err.totp_required": "Enter the code from your authenticator app.",
  "err.totp_invalid": "That code is not right. Check your phone's clock.",
  "err.totp_enabled": "Two-step sign-in is already on.",
  "err.already_decided": "This receipt was already reviewed.",
  "err.last_admin": "The last admin cannot be removed.",
  "err.cannot_delete_self": "You cannot remove yourself.",
  "err.backup_missing": "Choose a backup file.",
  "err.backup_invalid": "That backup file is not valid.",
};
