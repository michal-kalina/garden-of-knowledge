import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Garden of Knowledge",
  description:
    "A self-hosted RAG knowledge base: upload documents, ask questions, get cited answers.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className="font-sans antialiased">{children}</body>
    </html>
  );
}
