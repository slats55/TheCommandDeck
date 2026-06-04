"use client";

import Link from "next/link";
import {
  Activity,
  ArrowRight,
  ClipboardCheck,
  Globe,
  ShieldCheck,
  Terminal,
} from "lucide-react";
import { useAuthStore } from "@multica/core/auth";
import { buttonVariants } from "@multica/ui/components/ui/button";

/**
 * Public front door for the self-hosted CommandDeck instance.
 *
 * Deliberately minimal and token-driven so it reads as part of the same product
 * as the authenticated Command Deck dashboard. Every claim here maps to a real,
 * delivered capability — there is no marketing copy for unbuilt features.
 *
 * Lives in the `(landing)` route tree, which forces a light palette via
 * `.landing-light` (see apps/web/app/custom.css). Using semantic tokens keeps
 * the page in lockstep with that palette instead of hardcoding colors.
 */

interface Capability {
  icon: typeof Terminal;
  title: string;
  body: string;
}

// Only capabilities the product actually ships. If a capability is removed or
// not yet real, it does not belong on the public front door.
const CAPABILITIES: Capability[] = [
  {
    icon: Terminal,
    title: "Approved Commands",
    body: "Run only allowlisted, bounded commands against your runtimes — never arbitrary shell.",
  },
  {
    icon: Activity,
    title: "Runtime Health",
    body: "See each runtime's true online, stale, or offline state from live daemon heartbeats.",
  },
  {
    icon: Globe,
    title: "Preview Registry",
    body: "Track the lifecycle and reachability of your self-hosted previews.",
  },
  {
    icon: ClipboardCheck,
    title: "Workflow Evidence",
    body: "Keep structured execution records with safe, truthful status progression.",
  },
];

export function CommandDeckLanding() {
  const user = useAuthStore((s) => s.user);
  // Authenticated visitors are redirected into their workspace by
  // RedirectIfAuthenticated, but the brief pre-redirect render (and any case
  // where the redirect is still resolving) should still offer a coherent entry.
  const primaryHref = user ? "/" : "/login";
  const primaryLabel = user ? "Open CommandDeck" : "Sign in";

  return (
    <div className="relative flex min-h-full flex-col bg-background text-foreground">
      {/* Subtle top backdrop — token-driven, no hardcoded colors. */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 top-0 h-[420px] bg-gradient-to-b from-muted/50 to-transparent"
      />

      <header className="relative z-10 border-b border-border/60">
        <div className="mx-auto flex h-16 w-full max-w-5xl items-center justify-between px-6">
          <Link
            href="/"
            aria-label="CommandDeck home"
            className="flex items-center gap-2 rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <Terminal className="size-5 text-primary" aria-hidden />
            <span className="text-base font-semibold tracking-tight">
              CommandDeck
            </span>
          </Link>
          <Link href={primaryHref} className={buttonVariants({ size: "sm" })}>
            {primaryLabel}
          </Link>
        </div>
      </header>

      <main className="relative z-10 flex-1">
        <section className="mx-auto w-full max-w-3xl px-6 pb-16 pt-20 text-center sm:pt-28">
          <span className="inline-flex items-center gap-1.5 rounded-full border border-border bg-muted/50 px-3 py-1 text-xs font-medium text-muted-foreground">
            <ShieldCheck className="size-3.5" aria-hidden />
            Self-hosted operator control plane
          </span>

          <h1 className="mt-6 text-balance text-4xl font-semibold tracking-tight text-foreground sm:text-5xl">
            Secure control for agent-assisted development.
          </h1>

          <p className="mx-auto mt-5 max-w-xl text-pretty text-base leading-7 text-muted-foreground">
            Run approved commands, monitor runtimes, track previews, and record
            workflow evidence from one self-hosted CommandDeck control plane.
          </p>

          <div className="mt-8 flex items-center justify-center">
            <Link
              href={primaryHref}
              className={buttonVariants({ size: "lg" })}
            >
              {primaryLabel}
              <ArrowRight className="size-4" aria-hidden />
            </Link>
          </div>
        </section>

        <section
          aria-labelledby="capabilities-heading"
          className="mx-auto w-full max-w-5xl px-6 pb-16"
        >
          <h2 id="capabilities-heading" className="sr-only">
            Capabilities
          </h2>
          <div className="grid gap-4 sm:grid-cols-2">
            {CAPABILITIES.map(({ icon: Icon, title, body }) => (
              <div
                key={title}
                className="rounded-xl border border-border bg-card p-5 text-left"
              >
                <div className="flex size-9 items-center justify-center rounded-lg bg-muted text-foreground">
                  <Icon className="size-5" aria-hidden />
                </div>
                <h3 className="mt-4 text-sm font-semibold text-foreground">
                  {title}
                </h3>
                <p className="mt-1.5 text-sm leading-6 text-muted-foreground">
                  {body}
                </p>
              </div>
            ))}
          </div>
        </section>

        <section className="mx-auto w-full max-w-5xl px-6 pb-20">
          <div className="flex flex-col items-center gap-3 rounded-xl border border-border bg-muted/30 px-6 py-8 text-center sm:flex-row sm:justify-center sm:text-left">
            <ShieldCheck className="size-5 shrink-0 text-foreground" aria-hidden />
            <p className="text-sm font-medium text-foreground">
              Allowlisted commands only. Workspace-scoped evidence. No arbitrary
              shell access.
            </p>
          </div>
        </section>
      </main>

      <footer className="relative z-10 mt-auto border-t border-border/60">
        <div className="mx-auto flex w-full max-w-5xl flex-col items-center justify-between gap-3 px-6 py-8 sm:flex-row">
          <div className="flex items-center gap-2">
            <Terminal className="size-4 text-primary" aria-hidden />
            <span className="text-sm font-semibold tracking-tight">
              CommandDeck
            </span>
          </div>
          <p className="text-xs text-muted-foreground">
            Secure, self-hosted operator control plane.
          </p>
        </div>
      </footer>
    </div>
  );
}
