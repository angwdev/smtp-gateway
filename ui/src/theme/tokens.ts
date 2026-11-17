// The single source of the palette. index.css repeats these values so the
// first paint is correct before any script runs; tokens.test.ts keeps the two
// in step. Values and contrast ratios are documented in DESIGN.md.
export const tokens = {
  light: {
    paper: '#faf5ef',
    sheet: '#fefcfa',
    sunken: '#f3ede5',
    hairline: '#ddd4cc',
    control: '#90847a',
    ink: '#281c17',
    ink2: '#584a43',
    muted: '#73665f',
    accent: '#c22823',
    accentHover: '#a91515',
    onAccent: '#fefbf8',
    sent: '#206b38',
    rejected: '#84550f',
    failed: '#a51d2b',
    focus: '#0053a5',
  },
  dark: {
    paper: '#18130e',
    sheet: '#201a16',
    sunken: '#120d09',
    hairline: '#3b342e',
    control: '#7e7269',
    ink: '#ede7df',
    ink2: '#c5bcb3',
    muted: '#a69d94',
    accent: '#f68678',
    accentHover: '#ff9f92',
    onAccent: '#1a0e0d',
    sent: '#76cf8a',
    rejected: '#edbb64',
    failed: '#fe8a88',
    focus: '#87bafd',
  },
} as const;

export type TokenName = keyof typeof tokens.light;
export type Scheme = keyof typeof tokens;
