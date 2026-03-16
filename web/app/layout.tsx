import type { Metadata } from "next";
import "./styles.css";

export const metadata: Metadata = {
  title: "Permission Aware Search Demo",
  description: "Go + SQLite + SuperTokens-style auth headers"
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
