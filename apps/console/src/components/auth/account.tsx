"use client";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { LogOut, LoaderCircle } from "lucide-react";
import { mutate } from "@/lib/client-api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
} from "@/components/ui/field";
import { ErrorNotice, SuccessNotice } from "@/components/shared/feedback";

export function AccountForm() {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  return (
    <div className="form-section">
      <h2 className="text-lg font-semibold">Change password</h2>
      <form
        onSubmit={async (event) => {
          event.preventDefault();
          const form = event.currentTarget;
          const data = new FormData(form);
          setError(null);
          setSuccess(null);
          if (data.get("password") !== data.get("confirm")) {
            setError("The new passwords do not match.");
            return;
          }
          setBusy(true);
          try {
            await mutate("/v1/auth/password", {
              currentPassword: data.get("current"),
              password: data.get("password"),
            });
            form.reset();
            setSuccess(
              "Password changed. Your other sessions and MCP connections have been revoked.",
            );
            router.refresh();
          } catch (error) {
            setError((error as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="current">Current password</FieldLabel>
            <Input
              id="current"
              name="current"
              type="password"
              autoComplete="current-password"
              required
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="password">New password</FieldLabel>
            <Input
              id="password"
              name="password"
              type="password"
              autoComplete="new-password"
              minLength={12}
              maxLength={256}
              required
            />
            <FieldDescription>
              At least 12 characters. Changing it revokes your other sessions
              and MCP connections.
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="confirm">Confirm new password</FieldLabel>
            <Input
              id="confirm"
              name="confirm"
              type="password"
              autoComplete="new-password"
              minLength={12}
              required
            />
          </Field>
          <ErrorNotice message={error} />
          <SuccessNotice message={success} />
          <Button type="submit" disabled={busy}>
            {busy ? (
              <LoaderCircle data-icon="inline-start" className="animate-spin" />
            ) : null}
            Change password
          </Button>
        </FieldGroup>
      </form>
      <div className="pt-4">
        <Button
          variant="outline"
          onClick={async () => {
            try {
              await mutate("/v1/auth/logout");
              window.location.assign("/sign-in");
            } catch (error) {
              setError((error as Error).message);
            }
          }}
        >
          <LogOut data-icon="inline-start" />
          Sign out
        </Button>
      </div>
    </div>
  );
}
