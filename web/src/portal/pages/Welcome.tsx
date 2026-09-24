import { Link } from "react-router";
import { Gamepad2, Gauge, LayoutDashboard, Sparkles } from "lucide-react";
import { useI18n } from "@shared/i18n";
import { Button, Card } from "@shared/ui";
import { useInfo } from "../data";
import { PlanCard } from "./Plans";

export function Welcome() {
  const { t } = useI18n();
  const info = useInfo().data;
  const features = [
    { icon: Gamepad2, t: "welcome.f1.t", s: "welcome.f1.s" },
    { icon: Gauge, t: "welcome.f2.t", s: "welcome.f2.s" },
    { icon: LayoutDashboard, t: "welcome.f3.t", s: "welcome.f3.s" },
  ];
  return (
    <div className="animate-in">
      <section className="relative text-center pt-8 sm:pt-16 pb-10 max-w-3xl mx-auto">
        <span className="inline-flex items-center gap-2 h-8 px-3.5 rounded-full bg-primary-soft text-primary text-[13px] font-semibold">
          <Sparkles className="size-4" />
          {t("welcome.kicker")}
        </span>
        <h1 className="mt-6 text-[34px] sm:text-6xl font-black tracking-tight leading-[1.15]">
          {t("welcome.title.lead")}
          <br />
          <span className="gradient-text">{t("welcome.title.tail")}</span>
        </h1>
        <p className="mt-5 text-muted text-[15px] sm:text-lg leading-relaxed max-w-2xl mx-auto">{t("welcome.sub")}</p>
        <div className="mt-8 flex flex-col sm:flex-row gap-3 justify-center">
          {info?.signup_open !== false && (
            <Link to="/signup">
              <Button size="lg" className="w-full sm:w-auto px-8">
                {t("welcome.start")}
              </Button>
            </Link>
          )}
          <Link to="/login">
            <Button size="lg" variant="secondary" className="w-full sm:w-auto px-8">
              {t("welcome.login")}
            </Button>
          </Link>
        </div>
        {info?.signup_open === false && <p className="mt-4 text-sm text-muted">{t("welcome.closed")}</p>}
      </section>

      <section className="grid sm:grid-cols-3 gap-3 sm:gap-4">
        {features.map((f) => (
          <Card key={f.t} className="p-5">
            <div className="size-11 rounded-2xl gradient-bg text-white grid place-items-center shadow-[0_10px_24px_-10px_var(--primary)]">
              <f.icon className="size-5" />
            </div>
            <div className="font-bold mt-4">{t(f.t)}</div>
            <div className="text-sm text-muted mt-1 leading-relaxed">{t(f.s)}</div>
          </Card>
        ))}
      </section>

      {!!info?.plans.length && (
        <section className="mt-14 mb-6">
          <h2 className="text-xl font-extrabold mb-4">{t("welcome.plans")}</h2>
          <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {info.plans.map((p) => (
              <PlanCard key={p.id} plan={p} currency={info.currency} action={
                <Link to="/signup">
                  <Button block variant="soft">{t("welcome.start")}</Button>
                </Link>
              } />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
