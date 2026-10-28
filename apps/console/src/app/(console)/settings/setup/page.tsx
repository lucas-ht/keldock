import Link from "next/link";
import { ArrowUpRight, BookOpen, ShieldCheck } from "lucide-react";
import { api, requireAdmin } from "@/lib/server-api";
import { Refresh } from "@/components/layout/refresh";
import { Status } from "@/components/shared/status";
import { buttonVariants } from "@/components/ui/button";

type Setup = {
  clusterId: string;
  projectCount: number;
  steps: {
    id: string;
    title: string;
    state: string;
    detail: string;
    href?: string;
  }[];
};

export default async function SetupPage() {
  await requireAdmin();
  const setup = await api<Setup>("/v1/installation/onboarding");
  return (
    <>
      <div className="page-heading">
        <div>
          <p className="eyebrow">Self-hosted alpha</p>
          <h1>Installation setup</h1>
          <p>
            Your infrastructure, local accounts, and explicit capability
            choices.
          </p>
        </div>
        <Refresh automatic />
      </div>
      <div className="panel">
        <div className="panel-toolbar">
          <div>
            <h2 className="font-semibold">
              From installation to a healthy environment
            </h2>
            <p className="muted text-sm">
              {setup.clusterId} · {setup.projectCount} configured{" "}
              {setup.projectCount === 1 ? "project" : "projects"}
            </p>
          </div>
        </div>
        <ol className="divide-y divide-border">
          {setup.steps.map((step, index) => (
            <li className="flex gap-4 p-5" key={step.id}>
              <span className="flex size-7 shrink-0 items-center justify-center rounded-full border border-border text-xs text-muted-foreground">
                {index + 1}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <h3 className="font-medium">{step.title}</h3>
                  <Status state={step.state} />
                </div>
                <p className="muted mt-2 max-w-3xl break-words text-sm">
                  {step.detail}
                </p>
                {step.href && (
                  <Link
                    className="mt-3 inline-flex items-center gap-1 text-sm underline underline-offset-4"
                    href={step.href}
                  >
                    Open {step.title.toLowerCase()}
                    <ArrowUpRight className="size-3" />
                  </Link>
                )}
              </div>
            </li>
          ))}
        </ol>
      </div>
      <div className="mt-6 grid gap-5 lg:grid-cols-2">
        <section className="panel p-5">
          <h2 className="flex items-center gap-2 font-semibold">
            <ShieldCheck className="size-4" />
            Recovery is an operator procedure
          </h2>
          <p className="muted mt-3 text-sm">
            Keep an encrypted snapshot outside the installation. Restore to an
            empty database, compare surviving cluster checkpoints, review
            account access, then enroll new identities. This page does not
            certify a backup or restore.
          </p>
        </section>
        <section className="panel p-5">
          <h2 className="flex items-center gap-2 font-semibold">
            <BookOpen className="size-4" />
            Follow the installation guide
          </h2>
          <p className="muted my-3 text-sm">
            Use the guide and release checksums matching your installed version.
            Optional PostgreSQL requires separate controller, storage, backup
            and restore checks.
          </p>
          <a
            className={buttonVariants({ variant: "outline", size: "sm" })}
            href="https://github.com/lucas-ht/keldock/blob/main/docs/runtime/alpha/README.md"
            target="_blank"
            rel="noreferrer"
          >
            Installation and recovery guide
            <ArrowUpRight data-icon="inline-end" />
          </a>
        </section>
      </div>
    </>
  );
}
