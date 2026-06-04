import type { Metadata } from "next";
import { CommandDeckLanding } from "@/features/landing/components/commanddeck-landing";
import { RedirectIfAuthenticated } from "@/features/landing/components/redirect-if-authenticated";

export const metadata: Metadata = {
  title: {
    absolute: "CommandDeck — Secure control for agent-assisted development",
  },
  description:
    "Self-hosted operator control plane: run approved commands, monitor runtimes, track previews, and record workflow evidence.",
  openGraph: {
    title: "CommandDeck — Secure control for agent-assisted development",
    description:
      "Run approved commands, monitor runtimes, and track previews from one self-hosted control plane.",
    url: "/",
  },
  alternates: {
    canonical: "/",
  },
};

export default function LandingPage() {
  return (
    <>
      <RedirectIfAuthenticated />
      <CommandDeckLanding />
    </>
  );
}
