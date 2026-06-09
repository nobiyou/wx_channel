import nextCoreWebVitals from "eslint-config-next/core-web-vitals";

const config = [
  ...nextCoreWebVitals,
  {
    ignores: ["dist/**", ".next/**", "next-env.d.ts"]
  }
];

export default config;
