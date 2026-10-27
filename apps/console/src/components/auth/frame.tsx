import { Brand } from "@/components/layout/shell";
export function AuthFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="auth-shell">
      <aside className="auth-intro">
        <Brand />
        <div>
          <h2>
            Your applications.
            <br />
            Your infrastructure.
          </h2>
          <p>
            Follow every deployment from selected release to observed health,
            with control that stays in your hands.
          </p>
        </div>
        <span className="text-xs text-[var(--rail-muted)]">
          Free and open source · Customer-controlled infrastructure
        </span>
      </aside>
      <main className="auth-main">
        <div className="auth-form">{children}</div>
      </main>
    </div>
  );
}
