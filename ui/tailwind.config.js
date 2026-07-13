/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{vue,ts}"],
  darkMode: "media",
  theme: {
    extend: {
      colors: {
        surface: "#14161a",
        panel: "#1b1e23",
        border: "#262a30",
      },
    },
  },
  plugins: [],
};
