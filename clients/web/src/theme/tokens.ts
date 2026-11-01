export const lightTokens = {
  '--primary-color': '#007aff', // Apple Blue — filled controls only
  '--bg-color': '#f2f2f7', // System background
  '--bg-paper': '#ffffff', // Card/Surface background
  '--surface-raised': '#ffffff', // Bubbles and timeline tracks
  '--surface-sunken': '#f2f2f7', // Inset wells (packet log, JSON editor)
  '--text-primary': '#1d1d1f', // SF Pro color — 16.83:1 on paper
  '--text-secondary': '#6c6c70', // 5.23:1 on paper; safe for informative text
  '--text-tertiary': '#8e8e93', // 3.62:1 — decorative labels only
  '--border-color': 'rgba(0, 0, 0, 0.08)', // Natural contrast
  '--border-strong': 'rgba(0, 0, 0, 0.16)',
  '--accent-text': '#0b62c4', // 5.90:1 — accent used as text
  '--signal-ok': '#248a3d', // 4.40:1
  '--signal-warn': '#b25000', // 5.20:1
  '--signal-danger': '#d70015', // 5.38:1
  '--signal-ok-fill': '#34c759', // dots and bars only, never text
  '--signal-warn-fill': '#ff9500',
  '--signal-danger-fill': '#ff3b30',
  '--spacing-unit': '8px',
};

export const darkTokens = {
  '--primary-color': '#0a84ff', // Apple Blue dark
  '--bg-color': '#000000',
  '--bg-paper': '#1c1c1e', // Apple Dark Gray
  '--surface-raised': '#2c2c2e',
  '--surface-sunken': '#141415',
  '--text-primary': '#ffffff', // 17.01:1 on paper
  '--text-secondary': '#8e8e93', // 5.22:1 on paper
  '--text-tertiary': '#6c6c70', // decorative labels only
  '--border-color': 'rgba(255, 255, 255, 0.15)',
  '--border-strong': 'rgba(255, 255, 255, 0.28)',
  '--accent-text': '#0a84ff', // 4.66:1 on paper
  '--signal-ok': '#30d158', // 8.42:1
  '--signal-warn': '#ff9f0a', // 8.28:1
  '--signal-danger': '#ff453a', // 4.99:1
  '--signal-ok-fill': '#30d158',
  '--signal-warn-fill': '#ff9f0a',
  '--signal-danger-fill': '#ff453a',
  '--spacing-unit': '8px',
};

/**
 * Type scale. 12px is the floor for the data surface; body copy never goes
 * below 13px. Weights stay at Regular or above — light weights are unreadable
 * at these sizes.
 */
export const typeScale = {
  pageTitle: { fontSize: 17, fontWeight: 600, lineHeight: 1.3 },
  body: { fontSize: 15, fontWeight: 400, lineHeight: 1.55 },
  panelTitle: { fontSize: 13, fontWeight: 600, lineHeight: 1.4 },
  secondary: { fontSize: 13, fontWeight: 400, lineHeight: 1.45 },
  data: { fontSize: 12, fontWeight: 400, lineHeight: 1.5 },
  eyebrow: { fontSize: 11, fontWeight: 600, lineHeight: 1.2, letterSpacing: '0.04em' },
} as const;

export const monoFont =
  'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace';

/** 4px base grid. */
export const space = [0, 4, 8, 12, 16, 24, 32] as const;

export const radius = {
  bubble: 18,
  bubbleTail: 6,
  panel: 12,
  control: 10,
  track: 4,
} as const;
