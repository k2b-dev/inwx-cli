import { defineFibel } from "@valentinkolb/fibel";

export default defineFibel({
  title: "inwx CLI",
  description: "Documentation for the unofficial community CLI for INWX DNS.",
  locales: [{ code: "en", label: "English" }],
  defaultLocale: "en",
  headerLinks: [
    { label: "Commands", value: "/dns-read" },
    { label: "Safe changes", value: "/dns-mutations" },
    { label: "GitHub", value: "https://github.com/k2b-dev/inwx-cli" },
  ],
  footerLinks: [
    {
      label: "Unofficial community project — not affiliated with INWX GmbH",
      value: "https://www.inwx.com/",
    },
  ],
});
