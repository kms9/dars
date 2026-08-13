import type { Metadata, Viewport } from "next";
import { Inter } from "next/font/google";
import { ThemeProvider } from "@/components/theme-provider";
import { cn } from "@dars/ui/lib/utils";
import { WebProviders } from "@/components/web-providers";
import { resolveBrowserApiBaseUrl, resolveBrowserWsUrl } from "@/config/runtime-urls";
import "./globals.css";

const inter = Inter({ subsets: ["latin"], variable: "--font-inter" });

export const viewport: Viewport = { width: "device-width", initialScale: 1 };
export const metadata: Metadata = {
  title: { default: "DARS Lightweight Runtime", template: "%s | DARS" },
  description: "A lightweight runtime for agents, squads, direct chat, and runs.",
  robots: { index: false, follow: false },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning className={cn("h-full antialiased font-sans", inter.variable)}>
      <body className="h-full overflow-hidden">
        <ThemeProvider>
          <WebProviders apiBaseUrl={resolveBrowserApiBaseUrl(process.env)} wsUrl={resolveBrowserWsUrl(process.env)}>
            {children}
          </WebProviders>
        </ThemeProvider>
      </body>
    </html>
  );
}
