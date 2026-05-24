import type { Config } from 'tailwindcss';

// shadcn/ui new-york baseline (admin/guru). Colours mapped to CSS variables
// in `app/globals.css`. Siswa namespace adds an extra palette under the
// `siswa-*` prefix that consumes its own CSS vars (also in globals.css).
const config: Config = {
  darkMode: ['class'],
  content: [
    './app/**/*.{ts,tsx}',
    './components/**/*.{ts,tsx}',
  ],
  theme: {
    container: {
      center: true,
      padding: '1rem',
      screens: {
        '2xl': '1400px',
      },
    },
    extend: {
      colors: {
        border: 'hsl(var(--border))',
        input: 'hsl(var(--input))',
        ring: 'hsl(var(--ring))',
        background: 'hsl(var(--background))',
        foreground: 'hsl(var(--foreground))',
        primary: {
          DEFAULT: 'hsl(var(--primary))',
          foreground: 'hsl(var(--primary-foreground))',
        },
        secondary: {
          DEFAULT: 'hsl(var(--secondary))',
          foreground: 'hsl(var(--secondary-foreground))',
        },
        destructive: {
          DEFAULT: 'hsl(var(--destructive))',
          foreground: 'hsl(var(--destructive-foreground))',
        },
        muted: {
          DEFAULT: 'hsl(var(--muted))',
          foreground: 'hsl(var(--muted-foreground))',
        },
        accent: {
          DEFAULT: 'hsl(var(--accent))',
          foreground: 'hsl(var(--accent-foreground))',
        },
        card: {
          DEFAULT: 'hsl(var(--card))',
          foreground: 'hsl(var(--card-foreground))',
        },
        popover: {
          DEFAULT: 'hsl(var(--popover))',
          foreground: 'hsl(var(--popover-foreground))',
        },
        // Siswa pastel pop palette — only consumed inside `.siswa-theme`
        // wrappers. Safe to use anywhere via `bg-siswa-yellow`, etc.
        siswa: {
          bg: 'hsl(var(--siswa-bg))',
          surface: 'hsl(var(--siswa-surface))',
          text: 'hsl(var(--siswa-text))',
          'text-muted': 'hsl(var(--siswa-text-muted))',
          border: 'hsl(var(--siswa-border))',
          'border-soft': 'hsl(var(--siswa-border-soft))',
          yellow: 'hsl(var(--siswa-yellow))',
          pink: 'hsl(var(--siswa-pink))',
          blue: 'hsl(var(--siswa-blue))',
          green: 'hsl(var(--siswa-green))',
          lavender: 'hsl(var(--siswa-lavender))',
          cream: 'hsl(var(--siswa-cream))',
          success: 'hsl(var(--siswa-success))',
          warning: 'hsl(var(--siswa-warning))',
          danger: 'hsl(var(--siswa-danger))',
          // Section aliases for semantic usage.
          materi: 'hsl(var(--siswa-section-materi))',
          latihan: 'hsl(var(--siswa-section-latihan))',
          ulangan: 'hsl(var(--siswa-section-ulangan))',
          tugas: 'hsl(var(--siswa-section-tugas))',
          nilai: 'hsl(var(--siswa-section-nilai))',
          umum: 'hsl(var(--siswa-section-umum))',
        },
      },
      borderRadius: {
        lg: 'var(--radius)',
        md: 'calc(var(--radius) - 2px)',
        sm: 'calc(var(--radius) - 4px)',
        siswa: 'var(--siswa-radius)',
        'siswa-lg': 'var(--siswa-radius-lg)',
      },
      fontFamily: {
        sans: ['var(--font-inter)', 'Inter', 'ui-sans-serif', 'system-ui', '-apple-system'],
        display: ['var(--font-space-grotesk)', 'Space Grotesk', 'ui-sans-serif', 'system-ui'],
        mono: ['JetBrains Mono', 'ui-monospace', 'SFMono-Regular'],
      },
    },
  },
  plugins: [require('tailwindcss-animate')],
};

export default config;
