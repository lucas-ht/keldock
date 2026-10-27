"use client";
import { useEffect, useTransition } from "react";
import { useRouter } from "next/navigation";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";

export function Refresh({ automatic = false }: { automatic?: boolean }) {
  const router = useRouter();
  const [pending, start] = useTransition();
  useEffect(() => {
    if (!automatic) return;
    const timer = setInterval(() => {
      if (!pending && document.visibilityState === "visible")
        start(() => router.refresh());
    }, 10000);
    return () => clearInterval(timer);
  }, [automatic, pending, router]);
  return (
    <Button
      variant="outline"
      disabled={pending}
      onClick={() => start(() => router.refresh())}
    >
      <RefreshCw
        data-icon="inline-start"
        className={pending ? "animate-spin" : ""}
      />
      {pending ? "Refreshing" : "Refresh"}
    </Button>
  );
}
