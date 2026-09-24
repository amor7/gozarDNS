import { useState } from "react";
import { CircleCheck, CircleX, Gamepad2, Lightbulb, Monitor, Radar, Router, Smartphone, TriangleAlert } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { Button, Card, CopyButton, cn } from "@shared/ui";
import { useMe } from "../data";
import { guides, guideText } from "../guides";

type Result = { seen: boolean; same_ip: boolean; from: string; you: string };

const icons = { console: Gamepad2, phone: Smartphone, desktop: Monitor, router: Router };

export function Setup() {
  const { t, lang } = useI18n();
  const me = useMe().data;
  const dns = me?.dns || "";
  const [active, setActive] = useState(guides[0].id);
  const g = guides.find((x) => x.id === active)!;

  return (
    <div className="space-y-4 animate-in">
      <div>
        <h1 className="text-2xl font-extrabold">{t("setup.title")}</h1>
        <p className="text-muted mt-1">{t("setup.sub")}</p>
      </div>

      <Card className="p-5 sm:p-6 flex flex-col sm:flex-row items-center gap-4">
        <div className="flex-1 text-center sm:text-start">
          <div className="text-xs text-muted font-semibold">{t("dns.title")}</div>
          <div className="ltr num text-4xl font-black gradient-text mt-1 tracking-wide">{dns}</div>
        </div>
        <CopyButton text={dns} size="md" className="w-full sm:w-auto" />
      </Card>

      <ProbeCard />

      <Card className="overflow-hidden">
        <div className="flex gap-2 overflow-x-auto p-3 border-b border-line [scrollbar-width:none]">
          {guides.map((x) => {
            const Icon = icons[x.icon];
            return (
              <button key={x.id} onClick={() => setActive(x.id)}
                className={cn("shrink-0 h-10 px-3.5 rounded-xl text-sm font-semibold flex items-center gap-2 transition-all",
                  active === x.id ? "gradient-bg text-white shadow-[0_8px_20px_-10px_var(--primary)]" : "bg-surface-2 text-muted hover:text-fg")}>
                <Icon className="size-4" />
                {x.name[lang]}
              </button>
            );
          })}
        </div>
        <div className="p-5 sm:p-6" key={g.id}>
          <ol className="space-y-3">
            {g.steps.map((s, i) => (
              <li key={i} className="flex gap-3.5 animate-in" style={{ animationDelay: `${i * 40}ms`, animationFillMode: "backwards" }}>
                <span className="size-7 shrink-0 rounded-full bg-primary-soft text-primary grid place-items-center text-[13px] font-bold num">{(i + 1).toLocaleString(lang === "fa" ? "fa-IR" : "en-US")}</span>
                <span className="pt-0.5 leading-relaxed text-[15px]">
                  <StepText text={guideText(s, lang, dns)} dns={dns} />
                </span>
              </li>
            ))}
          </ol>
          {g.note && (
            <div className="mt-5 flex gap-2.5 rounded-2xl bg-warning-soft text-[13px] p-3.5 leading-relaxed">
              <TriangleAlert className="size-4 shrink-0 mt-0.5 text-warning" />
              <span>{g.note[lang]}</span>
            </div>
          )}
        </div>
      </Card>

      <Card className="p-5 sm:p-6">
        <div className="flex items-center gap-2 font-bold mb-3">
          <Lightbulb className="size-5 text-warning" />
          {t("setup.tips")}
        </div>
        <ul className="space-y-2.5 text-sm text-muted leading-relaxed list-disc ps-5 marker:text-primary">
          <li>{t("setup.tip1")}</li>
          <li>{t("setup.tip2")}</li>
          <li>{t("setup.tip3")}</li>
        </ul>
      </Card>
    </div>
  );
}

/** Highlights the DNS address wherever a step mentions it. */
function StepText({ text, dns }: { text: string; dns: string }) {
  if (!dns || !text.includes(dns)) return <>{text}</>;
  const parts = text.split(dns);
  return (
    <>
      {parts.map((p, i) => (
        <span key={i}>
          {p}
          {i < parts.length - 1 && <code className="ltr num inline-block px-2 py-0.5 mx-0.5 rounded-lg bg-primary-soft text-primary font-bold">{dns}</code>}
        </span>
      ))}
    </>
  );
}

/** Asks the browser to resolve a one-time name. Only this service's DNS
 *  answers it, so if the gate saw the lookup, the device is set up. */
function ProbeCard() {
  const { t } = useI18n();
  const [state, setState] = useState<"idle" | "running" | "done">("idle");
  const [res, setRes] = useState<Result | null>(null);

  const run = async () => {
    setState("running");
    setRes(null);
    const token = "t" + Array.from(crypto.getRandomValues(new Uint8Array(10)), (b) => b.toString(16).padStart(2, "0")).join("");
    const img = new Image();
    img.src = `https://${token}.check.gozar.internal/p.png`;
    let out: Result | null = null;
    for (let i = 0; i < 10; i++) {
      await new Promise((r) => setTimeout(r, 450));
      try {
        out = await api<Result>(`/probe/${token}`);
        if (out.seen) break;
      } catch {
        /* keep trying */
      }
    }
    setRes(out);
    setState("done");
  };

  let result: React.ReactNode = null;
  if (state === "done" && res) {
    if (res.seen && res.same_ip) result = <Outcome tone="success" title={t("setup.test.ok")} sub={t("setup.test.ok.s")} />;
    else if (res.seen) result = <Outcome tone="warning" title={t("setup.test.other")} sub={t("setup.test.other.s", { from: res.from, you: res.you })} />;
    else result = <Outcome tone="danger" title={t("setup.test.fail")} sub={t("setup.test.fail.s")} />;
  }

  return (
    <Card className="p-5 sm:p-6">
      <div className="flex flex-col sm:flex-row sm:items-center gap-4">
        <div className="size-12 rounded-2xl gradient-bg text-white grid place-items-center shrink-0 shadow-[0_10px_24px_-10px_var(--primary)]">
          <Radar className={cn("size-6", state === "running" && "animate-spin [animation-duration:2s]")} />
        </div>
        <div className="flex-1">
          <div className="font-bold">{t("setup.test.t")}</div>
          <div className="text-sm text-muted mt-0.5">{t("setup.test.s")}</div>
        </div>
        <Button onClick={run} loading={state === "running"} className="sm:w-auto w-full">
          {state === "running" ? t("setup.test.running") : t("setup.test.run")}
        </Button>
      </div>
      {result && <div className="mt-4">{result}</div>}
    </Card>
  );
}

function Outcome({ tone, title, sub }: { tone: "success" | "warning" | "danger"; title: string; sub: string }) {
  const cls = { success: "bg-success-soft text-success", warning: "bg-warning-soft text-warning", danger: "bg-danger-soft text-danger" }[tone];
  const Icon = tone === "success" ? CircleCheck : tone === "warning" ? TriangleAlert : CircleX;
  return (
    <div className={cn("rounded-2xl p-4 flex gap-3 animate-in", cls)}>
      <Icon className="size-5 shrink-0 mt-0.5" />
      <div>
        <div className="font-bold">{title}</div>
        <div className="text-sm text-fg/80 mt-1 leading-relaxed">{sub}</div>
      </div>
    </div>
  );
}
