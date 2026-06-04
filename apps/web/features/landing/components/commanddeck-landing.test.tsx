import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { AnchorHTMLAttributes, ReactNode } from "react";

// next/link is not globally mocked in apps/web tests; render it as a plain
// anchor so href assertions are straightforward and deterministic.
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: { href: string; children: ReactNode } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

const { authStateRef } = vi.hoisted(() => ({
  authStateRef: {
    state: { user: null as null | { id: string; email: string } },
  },
}));

// Web landing components read `useAuthStore((s) => s.user)`.
vi.mock("@multica/core/auth", () => {
  const useAuthStore = Object.assign(
    (selector: (s: typeof authStateRef.state) => unknown) =>
      selector(authStateRef.state),
    { getState: () => authStateRef.state },
  );
  return { useAuthStore };
});

import { CommandDeckLanding } from "./commanddeck-landing";

describe("CommandDeckLanding", () => {
  beforeEach(() => {
    authStateRef.state.user = null;
  });

  it("brands the page as CommandDeck", () => {
    render(<CommandDeckLanding />);
    // Wordmark appears in both the header and footer (and the product name in
    // the hero copy) — the page is unmistakably CommandDeck.
    expect(screen.getAllByText(/CommandDeck/).length).toBeGreaterThanOrEqual(2);
  });

  it("does not surface any leftover Multica product identity or old hero copy", () => {
    render(<CommandDeckLanding />);
    expect(screen.queryByText(/Multica/i)).not.toBeInTheDocument();
    expect(
      screen.queryByText(/Project Management for Human \+ Agent Teams/i),
    ).not.toBeInTheDocument();
  });

  it("offers a Sign in path to /login for unauthenticated visitors", () => {
    render(<CommandDeckLanding />);
    const signIn = screen.getAllByRole("link", { name: /sign in/i });
    expect(signIn.length).toBeGreaterThanOrEqual(1);
    for (const link of signIn) {
      expect(link).toHaveAttribute("href", "/login");
    }
  });

  it("renders only truthful, delivered capability copy", () => {
    render(<CommandDeckLanding />);
    expect(screen.getByText("Approved Commands")).toBeInTheDocument();
    expect(screen.getByText("Runtime Health")).toBeInTheDocument();
    expect(screen.getByText("Preview Registry")).toBeInTheDocument();
    expect(screen.getByText("Workflow Evidence")).toBeInTheDocument();
    expect(
      screen.getByText(
        /Allowlisted commands only\. Workspace-scoped evidence\. No arbitrary shell access\./i,
      ),
    ).toBeInTheDocument();
  });

  it("routes authenticated visitors to the app instead of the login screen", () => {
    authStateRef.state.user = { id: "u1", email: "operator@commanddeck.local" };
    render(<CommandDeckLanding />);

    const openLinks = screen.getAllByRole("link", { name: /open commanddeck/i });
    expect(openLinks.length).toBeGreaterThanOrEqual(1);
    for (const link of openLinks) {
      expect(link).toHaveAttribute("href", "/");
    }
    expect(
      screen.queryByRole("link", { name: /sign in/i }),
    ).not.toBeInTheDocument();
  });
});
