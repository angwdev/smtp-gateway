import { createTheme, type PaletteOptions } from '@mui/material/styles';

import { tokens, type Scheme } from './tokens';

const sans = "'Atkinson Hyperlegible Next Variable', system-ui, sans-serif";

function palette(scheme: Scheme): PaletteOptions {
  const t = tokens[scheme];
  return {
    primary: { main: t.accent, dark: t.accentHover, contrastText: t.onAccent },
    error: { main: t.failed },
    warning: { main: t.rejected },
    success: { main: t.sent },
    background: { default: t.paper, paper: t.sheet },
    text: { primary: t.ink, secondary: t.ink2, disabled: t.muted },
    divider: t.hairline,
  };
}

export const theme = createTheme({
  cssVariables: { colorSchemeSelector: 'class' },
  colorSchemes: { light: { palette: palette('light') }, dark: { palette: palette('dark') } },
  shape: { borderRadius: 2 },
  typography: {
    fontFamily: sans,
    h1: { fontSize: '1.5rem', fontWeight: 600, lineHeight: 1.25 },
    h2: { fontSize: '1.125rem', fontWeight: 600, lineHeight: 1.25 },
    body1: { fontSize: '1rem', lineHeight: 1.5 },
    body2: { fontSize: '0.875rem', lineHeight: 1.5 },
    caption: { fontSize: '0.8125rem', lineHeight: 1.4 },
    button: { textTransform: 'none', fontWeight: 600 },
  },
  components: {
    MuiButtonBase: { defaultProps: { disableRipple: true } },
    MuiButton: {
      defaultProps: { disableElevation: true },
      styleOverrides: { root: { minHeight: 44, paddingInline: 16 } },
    },
    MuiIconButton: { styleOverrides: { root: { minWidth: 44, minHeight: 44 } } },
    MuiPaper: { defaultProps: { elevation: 0 }, styleOverrides: { root: { backgroundImage: 'none' } } },
    MuiDialog: {
      defaultProps: { slotProps: { paper: { variant: 'outlined' } } },
    },
    MuiTextField: { defaultProps: { variant: 'outlined', fullWidth: true } },
    MuiOutlinedInput: {
      styleOverrides: {
        root: { backgroundColor: 'var(--sg-sheet)', minHeight: 44 },
        notchedOutline: { borderColor: 'var(--sg-control)' },
      },
    },
    MuiTooltip: { defaultProps: { arrow: false } },
  },
});
