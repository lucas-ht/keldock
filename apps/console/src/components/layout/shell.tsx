"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { useTheme } from "next-themes";
import {
  Boxes,
  Folder,
  GitBranch,
  LayoutGrid,
  Menu,
  Moon,
  Plug,
  ListChecks,
  Sun,
  Users,
} from "lucide-react";
import type { User } from "@/lib/contracts";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";

export function Brand() {
  return (
    <Link className="brand" href="/" aria-label="Keldock home">
      <span className="brand-mark" aria-hidden="true">
        <i />
        <i />
      </span>
      Keldock
    </Link>
  );
}
const navigation = [
  { href: "/", label: "Overview", icon: LayoutGrid },
  { href: "/projects", label: "Projects", icon: Folder },
  { href: "/clusters", label: "Clusters", icon: Boxes },
  { href: "/people", label: "People", icon: Users, admin: true },
  {
    href: "/settings/github",
    label: "GitHub delivery",
    icon: GitBranch,
    admin: true,
  },
  { href: "/settings/mcp", label: "MCP access", icon: Plug },
  { href: "/settings/setup", label: "Setup", icon: ListChecks, admin: true },
];
function Navigation({
  user,
  onNavigate,
}: {
  user: User;
  onNavigate?: () => void;
}) {
  const path = usePathname();
  return (
    <nav className="rail-nav" aria-label="Workspace navigation">
      {navigation
        .filter((item) => !item.admin || user.role === "admin")
        .map((item) => (
          <Link
            className="rail-link"
            key={item.href}
            href={item.href}
            onClick={onNavigate}
            aria-current={
              (item.href === "/" ? path === "/" : path.startsWith(item.href))
                ? "page"
                : undefined
            }
          >
            <item.icon />
            {item.label}
          </Link>
        ))}
    </nav>
  );
}
function Account({ user }: { user: User }) {
  return (
    <div className="rail-footer">
      <p className="rail-context">
        Customer-controlled infrastructure
        <br />
        {user.organizationId}
      </p>
      <Link className="account-link" href="/account">
        <span className="account-avatar" aria-hidden="true">
          {user.name
            .split(" ")
            .map((v) => v[0])
            .slice(0, 2)
            .join("")}
        </span>
        <span className="min-w-0">
          <span className="block truncate">{user.name}</span>
          <small>{user.role}</small>
        </span>
      </Link>
    </div>
  );
}
function ThemeButton() {
  const { resolvedTheme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  const dark = mounted && resolvedTheme === "dark";
  return (
    <Button
      variant="ghost"
      size="icon"
      aria-label={dark ? "Use light theme" : "Use dark theme"}
      onClick={() => setTheme(dark ? "light" : "dark")}
    >
      {dark ? <Sun /> : <Moon />}
    </Button>
  );
}
export function Shell({
  user,
  notice,
  children,
}: {
  user: User;
  notice?: string;
  children: React.ReactNode;
}) {
  const path = usePathname();
  const [open, setOpen] = useState(false);
  const current = navigation.find(
    (item) => item.href !== "/" && path.startsWith(item.href),
  );
  return (
    <div className="app-shell">
      <aside className="rail">
        <Brand />
        <Navigation user={user} />
        <Account user={user} />
      </aside>
      <div className="min-w-0">
        <header className="topbar">
          <Sheet open={open} onOpenChange={setOpen}>
            <SheetTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  className="mobile-menu md:hidden"
                  aria-label="Open navigation"
                />
              }
            >
              <Menu />
            </SheetTrigger>
            <SheetContent side="left">
              <SheetTitle className="sr-only">Workspace navigation</SheetTitle>
              <div className="rail h-full">
                <Brand />
                <Navigation user={user} onNavigate={() => setOpen(false)} />
                <Account user={user} />
              </div>
            </SheetContent>
          </Sheet>
          <span className="topbar-title">
            {path === "/account" ? "Account" : (current?.label ?? "Overview")}
          </span>
          <div className="topbar-tools">
            <span
              className="topbar-context text-xs text-muted-foreground"
              data-notice={Boolean(notice)}
            >
              {notice || "Self-hosted control plane"}
            </span>
            <ThemeButton />
          </div>
        </header>
        <main id="main" className="page-content">
          {children}
        </main>
      </div>
    </div>
  );
}
