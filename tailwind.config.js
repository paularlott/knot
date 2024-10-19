/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./web/src/**/*.{js,ts,jsx,tsx}",
    "./web/templates/**/*.tmpl",
    "./web/public_html/**/*.html",
  ],
  theme: {
    fontFamily: {
      'nunito': ['Nunito', 'sans-serif'],
      'jbmono': ['JetBrains Mono', 'monospace'],
    },
    extend: {
    },
  },
  plugins: [
    require('@tailwindcss/forms'),
  ],
  darkMode: 'class',
}

