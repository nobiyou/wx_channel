import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Hub Sites Frontend",
  description: "Sites 版 Hub 控制台前端"
};

export default function RootLayout({
  children
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
