"use client";
import { AuthFrame } from "@/components/auth/frame";
import { ErrorNotice } from "@/components/shared/feedback";
import { Button } from "@/components/ui/button";

export default function ControlPlaneError({
  reset,
}: {
  error: Error;
  reset: () => void;
}) {
  return (
    <AuthFrame>
      <h1>Control plane unavailable</h1>
      <p>
        The API could not load your workspace. Check its connection, then try
        again.
      </p>
      <div className="flex flex-col gap-5">
        <ErrorNotice message="Your last approved workloads remain under cluster reconciliation while the control plane reconnects." />
        <Button onClick={() => window.location.reload()}>Try again</Button>
      </div>
    </AuthFrame>
  );
}
