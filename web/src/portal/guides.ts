import type { Lang } from "@shared/i18n";

export type Guide = { id: string; name: { fa: string; en: string }; icon: "console" | "phone" | "desktop" | "router"; steps: { fa: string; en: string }[]; note?: { fa: string; en: string } };

// {dns} is replaced with the gate's address when shown.
export const guides: Guide[] = [
  {
    id: "ps5",
    name: { fa: "پلی‌استیشن ۵", en: "PlayStation 5" },
    icon: "console",
    steps: [
      { fa: "Settings › Network › Settings › Set Up Internet Connection", en: "Settings → Network → Settings → Set Up Internet Connection" },
      { fa: "روی شبکهٔ فعلی دکمهٔ Options را بزنید و Advanced Settings را انتخاب کنید", en: "Highlight your network, press Options and choose Advanced Settings" },
      { fa: "DNS Settings را روی Manual بگذارید", en: "Set DNS Settings to Manual" },
      { fa: "Primary DNS و Secondary DNS هر دو: {dns}", en: "Primary DNS and Secondary DNS: both {dns}" },
      { fa: "OK را بزنید و کنسول را یک بار ری‌استارت کنید", en: "Press OK, then restart the console once" },
    ],
  },
  {
    id: "ps4",
    name: { fa: "پلی‌استیشن ۴", en: "PlayStation 4" },
    icon: "console",
    steps: [
      { fa: "Settings › Network › Set Up Internet Connection", en: "Settings → Network → Set Up Internet Connection" },
      { fa: "Wi-Fi یا LAN، سپس Custom", en: "Choose Wi-Fi or LAN, then Custom" },
      { fa: "IP Address: Automatic — DHCP Host Name: Do Not Specify", en: "IP Address: Automatic — DHCP Host Name: Do Not Specify" },
      { fa: "DNS Settings: Manual — Primary و Secondary هر دو: {dns}", en: "DNS Settings: Manual — Primary and Secondary: both {dns}" },
      { fa: "MTU: Automatic — Proxy Server: Do Not Use", en: "MTU: Automatic — Proxy Server: Do Not Use" },
    ],
  },
  {
    id: "xbox",
    name: { fa: "ایکس‌باکس", en: "Xbox" },
    icon: "console",
    steps: [
      { fa: "Settings › General › Network settings › Advanced settings", en: "Settings → General → Network settings → Advanced settings" },
      { fa: "DNS settings › Manual", en: "DNS settings → Manual" },
      { fa: "Primary IPv4 DNS و Secondary IPv4 DNS هر دو: {dns}", en: "Primary IPv4 DNS and Secondary IPv4 DNS: both {dns}" },
      { fa: "با B برگردید تا تنظیمات ذخیره شود", en: "Press B to go back; the settings are saved" },
    ],
  },
  {
    id: "switch",
    name: { fa: "نینتندو سوییچ", en: "Nintendo Switch" },
    icon: "console",
    steps: [
      { fa: "System Settings › Internet › Internet Settings", en: "System Settings → Internet → Internet Settings" },
      { fa: "شبکهٔ خود را انتخاب کنید › Change Settings", en: "Pick your network → Change Settings" },
      { fa: "DNS Settings › Manual", en: "DNS Settings → Manual" },
      { fa: "Primary DNS و Secondary DNS هر دو: {dns} › Save", en: "Primary DNS and Secondary DNS: both {dns} → Save" },
    ],
  },
  {
    id: "android",
    name: { fa: "اندروید", en: "Android" },
    icon: "phone",
    steps: [
      { fa: "اول: Settings › Network › Private DNS را روی Off بگذارید", en: "First: Settings → Network → Private DNS → Off" },
      { fa: "Wi-Fi › روی شبکهٔ وصل‌شده بزنید › Edit / ⚙", en: "Wi-Fi → tap the connected network → Edit / ⚙" },
      { fa: "Advanced › IP settings را روی Static بگذارید", en: "Advanced → set IP settings to Static" },
      { fa: "DNS 1 و DNS 2 هر دو: {dns} › Save", en: "DNS 1 and DNS 2: both {dns} → Save" },
    ],
    note: {
      fa: "روی اینترنت سیم‌کارت، اندروید تنظیم DNS دستی ندارد؛ از اپی مثل «DNS Changer» استفاده کنید یا از Wi-Fi.",
      en: "On mobile data Android has no manual DNS setting; use a DNS-changer app or Wi-Fi.",
    },
  },
  {
    id: "iphone",
    name: { fa: "آیفون و آیپد", en: "iPhone & iPad" },
    icon: "phone",
    steps: [
      { fa: "Settings › Wi-Fi › دکمهٔ ⓘ کنار شبکه", en: "Settings → Wi-Fi → the ⓘ next to your network" },
      { fa: "Configure DNS › Manual", en: "Configure DNS → Manual" },
      { fa: "آدرس‌های قبلی را حذف کنید و Add Server: {dns}", en: "Delete the existing servers, then Add Server: {dns}" },
      { fa: "Save", en: "Save" },
    ],
  },
  {
    id: "windows",
    name: { fa: "ویندوز", en: "Windows" },
    icon: "desktop",
    steps: [
      { fa: "Settings › Network & Internet › Wi-Fi یا Ethernet › Hardware properties", en: "Settings → Network & Internet → Wi-Fi or Ethernet → Hardware properties" },
      { fa: "کنار DNS server assignment روی Edit بزنید › Manual", en: "Next to DNS server assignment, press Edit → Manual" },
      { fa: "IPv4 را روشن کنید؛ Preferred و Alternate DNS هر دو: {dns}", en: "Turn IPv4 on; Preferred and Alternate DNS: both {dns}" },
      { fa: "در مرورگر «Use secure DNS» را خاموش کنید", en: "Turn off “Use secure DNS” in your browser" },
    ],
  },
  {
    id: "mac",
    name: { fa: "مک", en: "macOS" },
    icon: "desktop",
    steps: [
      { fa: "System Settings › Network › Wi-Fi › Details", en: "System Settings → Network → Wi-Fi → Details" },
      { fa: "بخش DNS › با + آدرس {dns} را اضافه کنید", en: "DNS → press + and add {dns}" },
      { fa: "آدرس‌های دیگر را با − حذف کنید › OK", en: "Remove any other servers with − → OK" },
    ],
  },
  {
    id: "router",
    name: { fa: "مودم (همهٔ دستگاه‌ها)", en: "Router (every device)" },
    icon: "router",
    steps: [
      { fa: "وارد صفحهٔ مودم شوید (معمولاً 192.168.1.1)", en: "Open your router's page (usually 192.168.1.1)" },
      { fa: "بخش LAN یا DHCP Server را پیدا کنید", en: "Find the LAN or DHCP Server section" },
      { fa: "Primary و Secondary DNS هر دو: {dns}", en: "Primary and Secondary DNS: both {dns}" },
      { fa: "ذخیره کنید و دستگاه‌ها را یک بار از Wi-Fi قطع و وصل کنید", en: "Save, then reconnect your devices to Wi-Fi once" },
    ],
    note: {
      fa: "با تنظیم روی مودم، همهٔ دستگاه‌های خانه از سرویس استفاده می‌کنند و فقط آی‌پی مودم باید ثبت شود.",
      en: "Set on the router, every device at home uses the service and only the router's address needs registering.",
    },
  },
];

export const guideText = (s: { fa: string; en: string }, lang: Lang, dns: string) => s[lang].replaceAll("{dns}", dns);
