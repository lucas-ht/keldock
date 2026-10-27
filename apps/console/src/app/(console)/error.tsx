"use client";
import { ErrorNotice } from "@/components/shared/feedback";
import { Button } from "@/components/ui/button";
export default function ErrorPage({
  reset,
}: {
  error: Error;
  reset: () => void;
}) {
  return (
    <div className="form-section">
      <h1 className="text-2xl font-semibold">Workspace unavailable</h1>
      <ErrorNotice message="The control plane could not load this screen. Check the API connection and try again." />
      <Button onClick={() => window.location.reload()}>Try again</Button>
    </div>
  );
}
