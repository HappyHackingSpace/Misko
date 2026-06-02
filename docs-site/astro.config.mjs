// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// GitHub Pages project site: https://happyhackingspace.github.io/Misko
export default defineConfig({
  site: "https://happyhackingspace.github.io",
  base: "/Misko",
  integrations: [
    starlight({
      title: "Mişko",
      tagline: "Behavioral test management for laboratory mice",
      logo: { src: "./src/assets/logo.svg", alt: "Mişko" },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/HappyHackingSpace/Misko",
        },
      ],
      defaultLocale: "root",
      locales: {
        root: { label: "English", lang: "en" },
        tr: { label: "Türkçe", lang: "tr" },
      },
      sidebar: [
        {
          label: "Start here",
          translations: { tr: "Başlangıç" },
          items: [
            { label: "Overview", translations: { tr: "Genel bakış" }, slug: "overview" },
            { label: "Architecture", translations: { tr: "Mimari" }, slug: "architecture" },
          ],
        },
        {
          label: "Design",
          translations: { tr: "Tasarım" },
          items: [
            { label: "Domain model", translations: { tr: "Alan modeli" }, slug: "domain" },
            { label: "Integration (CV)", translations: { tr: "Entegrasyon (CV)" }, slug: "integration" },
            { label: "Roadmap", translations: { tr: "Yol haritası" }, slug: "roadmap" },
          ],
        },
      ],
    }),
  ],
});
