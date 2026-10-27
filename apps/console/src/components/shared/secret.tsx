"use client";
import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "./feedback";
export function SecretOutput({
  value,
  label,
}: {
  value: string;
  label: string;
}) {
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<string | null>(null);
  return (
    <div className="flex flex-col gap-3">
      <label className="font-medium">{label}</label>
      <pre className="secret-output" tabIndex={0}>
        {value}
      </pre>
      <div>
        <Button
          variant="outline"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setCopied(true);
              setError(null);
            } catch {
              setError(
                "Clipboard unavailable. Select and copy the value above.",
              );
            }
          }}
        >
          {copied ? (
            <Check data-icon="inline-start" />
          ) : (
            <Copy data-icon="inline-start" />
          )}
          {copied ? "Copied" : `Copy ${label.toLowerCase()}`}
        </Button>
      </div>
      <ErrorNotice message={error} />
    </div>
  );
}
