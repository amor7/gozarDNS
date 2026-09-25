import { useState } from "react";
import { CircleCheck, CircleX, TriangleAlert } from "lucide-react";
import { api } from "@shared/api";
import { useI18n } from "@shared/i18n";
import { cn } from "@shared/ui";

type Result = { seen: boolean; same_ip: boolean; from: string; you: string };

/** The setup check: have the browser resolve a one-time name that only the
 *  service's DNS answers, then ask the gate whether it saw that lookup. */
export function useProbe() {
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
  return { state, res, run };
}

export function ProbeResult({ res }: { res: Result | null }) {
  const { t } = useI18n();
  if (!res) return null;
  const [tone, title, sub] =
    res.seen && res.same_ip
      ? (["success", t("setup.test.ok"), t("setup.test.ok.s")] as const)
      : res.seen
        ? (["warning", t("setup.test.other"), t("setup.test.other.s", { from: res.from, you: res.you })] as const)
        : (["danger", t("setup.test.fail"), t("setup.test.fail.s")] as const);
  const cls = { success: "bg-success-soft text-success", warning: "bg-warning-soft text-warning", danger: "bg-danger-soft text-danger" }[tone];
  const Icon = tone === "success" ? CircleCheck : tone === "warning" ? TriangleAlert : CircleX;
  return (
    <div className={cn("rounded-2xl p-3.5 flex gap-3 animate-in", cls)}>
      <Icon className="size-5 shrink-0 mt-0.5" />
      <div>
        <div className="font-bold text-sm">{title}</div>
        <div className="text-[13px] text-fg/80 mt-1 leading-relaxed">{sub}</div>
      </div>
    </div>
  );
}
