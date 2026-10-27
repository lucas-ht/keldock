"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { LoaderCircle, LogIn } from "lucide-react";
import { mutate } from "@/lib/client-api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
} from "@/components/ui/field";
import { ErrorNotice } from "@/components/shared/feedback";

export function SignInForm({ next }: { next: string }) {
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  return (
    <form
      onSubmit={async (event) => {
        event.preventDefault();
        setError(null);
        setBusy(true);
        const data = new FormData(event.currentTarget);
        try {
          await mutate("/v1/auth/login", {
            email: data.get("email"),
            password: data.get("password"),
          });
          window.location.assign(next);
        } catch (error) {
          setError((error as Error).message);
          setBusy(false);
        }
      }}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor="email">Email address</FieldLabel>
          <Input
            id="email"
            name="email"
            type="email"
            autoComplete="username"
            required
            autoFocus
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="password">Password</FieldLabel>
          <Input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            required
          />
        </Field>
        <ErrorNotice message={error} />
        <Button type="submit" size="lg" disabled={busy}>
          {busy ? (
            <LoaderCircle data-icon="inline-start" className="animate-spin" />
          ) : (
            <LogIn data-icon="inline-start" />
          )}
          {busy ? "Signing in" : "Sign in"}
        </Button>
        <FieldDescription>
          Need an account or a password reset? Contact your installation
          administrator.
        </FieldDescription>
      </FieldGroup>
    </form>
  );
}

export function ActivateForm() {
  const router = useRouter();
  const [token, setToken] = useState("");
  const [info, setInfo] = useState<{
    name: string;
    email: string;
    kind: string;
    expiresAt: string;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    const fragment = window.location.hash.slice(1);
    const value = fragment.startsWith("token=")
      ? new URLSearchParams(fragment).get("token")
      : fragment;
    if (!value) {
      setError(
        "This link has no invitation token. Open the full link supplied by your administrator.",
      );
      return;
    }
    setToken(value);
    window.history.replaceState(null, "", window.location.pathname);
    mutate<typeof info>("/v1/auth/invitations/info", { token: value })
      .then(setInfo)
      .catch((error) => setError(error.message));
  }, []);
  if (!info)
    return (
      <>
        <p>
          {error
            ? "The invitation could not be opened."
            : "Checking your invitation…"}
        </p>
        <ErrorNotice message={error} />
      </>
    );
  return (
    <>
      <p className="mb-6 text-muted-foreground">
        {info.name}, set a password for <strong>{info.email}</strong>.
      </p>
      <form
        onSubmit={async (event) => {
          event.preventDefault();
          setError(null);
          const data = new FormData(event.currentTarget);
          if (data.get("password") !== data.get("confirm")) {
            setError("The passwords do not match.");
            return;
          }
          setBusy(true);
          try {
            await mutate("/v1/auth/activate", {
              token,
              password: data.get("password"),
            });
            router.replace("/");
            router.refresh();
          } catch (error) {
            setError((error as Error).message);
            setBusy(false);
          }
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="new-password">New password</FieldLabel>
            <Input
              id="new-password"
              name="password"
              type="password"
              autoComplete="new-password"
              minLength={12}
              maxLength={256}
              required
            />
            <FieldDescription>Use at least 12 characters.</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="confirm">Confirm password</FieldLabel>
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
          <Button size="lg" type="submit" disabled={busy}>
            {busy ? (
              <LoaderCircle data-icon="inline-start" className="animate-spin" />
            ) : null}
            {busy
              ? "Saving password"
              : info.kind === "reset"
                ? "Reset password"
                : "Activate account"}
          </Button>
        </FieldGroup>
      </form>
    </>
  );
}
