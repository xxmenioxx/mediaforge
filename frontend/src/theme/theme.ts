import { alpha, createTheme, type Theme } from '@mui/material/styles';

const selectedSurface = 'rgba(58, 43, 164, 0.56)';

export const mvforgeTokens = {
  colors: {
    canvas: '#04101d',
    workspace: '#061525',
    surface: '#091a2c',
    surfaceRaised: '#0c2035',
    surfaceInteractive: '#112a44',
    surfaceSelected: selectedSurface,
    text: {
      primary: '#e7eef8',
      secondary: '#9cb0c8',
      muted: '#8398b0',
      disabled: '#71849b',
    },
    border: {
      subtle: 'rgba(255,255,255,0.08)',
      default: '#15304a',
      strong: '#30465f',
    },
    accent: {
      primary: '#55a0ff',
      secondary: '#a78bfa',
      violet: '#6c4cf6',
      secondarySurface: '#27175d',
    },
    status: {
      success: '#00d99b',
      successSurface: '#073d35',
      warning: '#ffc43d',
      warningSurface: '#41320d',
      error: '#ff4e6a',
      errorSurface: '#481827',
      info: '#55a0ff',
      infoSurface: '#0b3152',
      neutral: '#71849b',
    },
  },
  typography: {
    body: '0.8rem',
    compactBody: '0.78rem',
    supporting: '0.75rem',
    tertiary: '0.72rem',
    sectionHeading: '0.8rem',
    pageHeading: '1.25rem',
  },
  density: {
    controlHeight: {
      compact: 32,
      normal: 36,
    },
    compactRowHeight: 42,
  },
  interaction: {
    hoverBackground: 'rgba(17, 42, 68, 0.72)',
    hoverBorder: '#58718c',
    selectedBackground: `linear-gradient(90deg, ${selectedSurface}, rgba(34, 41, 96, 0.36))`,
    selectedBorder: 'rgba(95, 75, 247, 0.48)',
    focus: '#6c4cf6',
  },
} as const;

export type MVForgeThemeTokens = typeof mvforgeTokens;

declare module '@mui/material/styles' {
  interface Theme {
    mvforge: MVForgeThemeTokens;
  }

  interface ThemeOptions {
    mvforge?: MVForgeThemeTokens;
  }
}

export function getMVForgeTokens(currentTheme: Partial<Theme> = {}): MVForgeThemeTokens {
  return currentTheme.mvforge ?? mvforgeTokens;
}

export const theme = createTheme({
  mvforge: mvforgeTokens,
  palette: {
    mode: 'dark',
    background: {
      default: mvforgeTokens.colors.canvas,
      paper: mvforgeTokens.colors.surface,
    },
    text: {
      primary: mvforgeTokens.colors.text.primary,
      secondary: mvforgeTokens.colors.text.secondary,
      disabled: mvforgeTokens.colors.text.disabled,
    },
    primary: {
      main: mvforgeTokens.colors.accent.primary,
    },
    secondary: {
      main: mvforgeTokens.colors.accent.secondary,
    },
    success: {
      main: mvforgeTokens.colors.status.success,
    },
    warning: {
      main: mvforgeTokens.colors.status.warning,
    },
    error: {
      main: mvforgeTokens.colors.status.error,
    },
    info: {
      main: mvforgeTokens.colors.status.info,
    },
    divider: mvforgeTokens.colors.border.subtle,
  },
  shape: {
    borderRadius: 8,
  },
  typography: {
    fontFamily:
      'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    h1: {
      fontSize: '2rem',
      fontWeight: 700,
      letterSpacing: 0,
    },
    h2: {
      fontSize: '1.35rem',
      fontWeight: 700,
      letterSpacing: 0,
    },
    h3: {
      fontSize: '1.1rem',
      fontWeight: 700,
      letterSpacing: 0,
    },
    button: {
      textTransform: 'none',
      fontWeight: 700,
      letterSpacing: 0,
    },
  },
  components: {
    MuiButtonBase: {
      styleOverrides: {
        root: {
          '&.Mui-focusVisible': {
            outline: `2px solid ${mvforgeTokens.interaction.focus}`,
            outlineOffset: 2,
          },
        },
      },
    },
    MuiButton: {
      styleOverrides: {
        root: {
          borderRadius: 6,
          minHeight: 40,
          paddingLeft: 18,
          paddingRight: 18,
          textTransform: 'none',
          '&:hover': { boxShadow: 'none' },
          '&.Mui-focusVisible': {
            outline: `2px solid ${mvforgeTokens.interaction.focus}`,
            outlineOffset: 2,
          },
          '[data-ui-density="compact"] &': {
            minHeight: mvforgeTokens.density.controlHeight.compact,
            paddingLeft: 10,
            paddingRight: 10,
            fontSize: mvforgeTokens.typography.compactBody,
            fontWeight: 600,
          },
          '&.MuiButton-sizeSmall .MuiButton-startIcon': { marginRight: 4 },
          '&.MuiButton-sizeSmall .MuiButton-startIcon > *': { fontSize: 16 },
          '[data-ui-density="compact"] & .MuiButton-startIcon': { marginRight: 4 },
          '[data-ui-density="compact"] & .MuiButton-startIcon > *': { fontSize: 16 },
        },
        sizeSmall: {
          minHeight: mvforgeTokens.density.controlHeight.compact,
          paddingLeft: 10,
          paddingRight: 10,
          fontSize: mvforgeTokens.typography.compactBody,
          fontWeight: 600,
        },
        contained: {
          boxShadow: 'none',
          '&:hover': { boxShadow: 'none' },
        },
        containedPrimary: {
          color: mvforgeTokens.colors.canvas,
          backgroundColor: mvforgeTokens.colors.accent.primary,
          '&:hover': {
            backgroundColor: mvforgeTokens.colors.accent.primary,
            filter: 'brightness(1.08)',
          },
          '&.Mui-disabled': {
            color: mvforgeTokens.colors.text.disabled,
            backgroundColor: alpha(mvforgeTokens.colors.accent.primary, 0.22),
          },
        },
        containedSecondary: {
          color: mvforgeTokens.colors.canvas,
          backgroundColor: mvforgeTokens.colors.accent.secondary,
          '&:hover': {
            backgroundColor: mvforgeTokens.colors.accent.secondary,
            filter: 'brightness(1.06)',
          },
          '&.Mui-disabled': {
            color: mvforgeTokens.colors.text.disabled,
            backgroundColor: alpha(mvforgeTokens.colors.accent.secondary, 0.22),
          },
        },
        containedError: {
          color: mvforgeTokens.colors.status.error,
          backgroundColor: mvforgeTokens.colors.status.errorSurface,
          '&:hover': {
            backgroundColor: mvforgeTokens.colors.status.errorSurface,
            filter: 'brightness(1.15)',
          },
        },
        outlined: {
          borderColor: mvforgeTokens.colors.border.default,
          color: mvforgeTokens.colors.text.secondary,
          '&:hover': {
            borderColor: mvforgeTokens.interaction.hoverBorder,
            backgroundColor: mvforgeTokens.interaction.hoverBackground,
          },
          '&.Mui-disabled': { borderColor: mvforgeTokens.colors.border.subtle },
        },
        outlinedPrimary: {
          borderColor: alpha(mvforgeTokens.colors.accent.primary, 0.55),
          color: mvforgeTokens.colors.accent.primary,
          '&:hover': {
            borderColor: mvforgeTokens.colors.accent.primary,
            backgroundColor: alpha(mvforgeTokens.colors.accent.primary, 0.08),
          },
        },
        outlinedSecondary: {
          borderColor: alpha(mvforgeTokens.colors.accent.secondary, 0.55),
          color: mvforgeTokens.colors.accent.secondary,
          '&:hover': {
            borderColor: mvforgeTokens.colors.accent.secondary,
            backgroundColor: alpha(mvforgeTokens.colors.accent.secondary, 0.08),
          },
        },
        outlinedError: {
          borderColor: alpha(mvforgeTokens.colors.status.error, 0.55),
          color: mvforgeTokens.colors.status.error,
          '&:hover': {
            borderColor: mvforgeTokens.colors.status.error,
            backgroundColor: mvforgeTokens.colors.status.errorSurface,
          },
        },
        textError: { color: mvforgeTokens.colors.status.error },
        text: {
          '&:hover': { backgroundColor: mvforgeTokens.interaction.hoverBackground },
        },
      },
    },
    MuiIconButton: {
      styleOverrides: {
        root: {
          borderRadius: 6,
          color: mvforgeTokens.colors.text.secondary,
          '&:hover': { backgroundColor: mvforgeTokens.interaction.hoverBackground },
          '&.Mui-focusVisible': {
            outline: `2px solid ${mvforgeTokens.interaction.focus}`,
            outlineOffset: 2,
          },
          '&.Mui-disabled': { color: mvforgeTokens.colors.text.disabled },
          '[data-ui-density="compact"] &': {
            minWidth: 40,
            minHeight: 40,
            padding: 8,
          },
        },
        sizeSmall: {
          minWidth: 40,
          minHeight: 40,
          padding: 8,
        },
      },
    },
    MuiInputBase: {
      styleOverrides: {
        root: {
          color: mvforgeTokens.colors.text.primary,
          fontSize: mvforgeTokens.typography.body,
        },
        input: {
          '&::placeholder': {
            color: mvforgeTokens.colors.text.muted,
            opacity: 1,
          },
          '&.Mui-disabled': {
            WebkitTextFillColor: mvforgeTokens.colors.text.disabled,
          },
        },
      },
    },
    MuiOutlinedInput: {
      styleOverrides: {
        root: {
          borderRadius: 6,
          backgroundColor: mvforgeTokens.colors.surface,
          '& .MuiOutlinedInput-notchedOutline': {
            borderColor: mvforgeTokens.colors.border.default,
          },
          '&:hover .MuiOutlinedInput-notchedOutline': {
            borderColor: mvforgeTokens.interaction.hoverBorder,
          },
          '&.Mui-focused': {
            outline: `2px solid ${alpha(mvforgeTokens.interaction.focus, 0.28)}`,
            outlineOffset: 1,
          },
          '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
            borderColor: mvforgeTokens.interaction.focus,
            borderWidth: 2,
          },
          '&.Mui-error .MuiOutlinedInput-notchedOutline': {
            borderColor: mvforgeTokens.colors.status.error,
          },
          '&.Mui-error.Mui-focused .MuiOutlinedInput-notchedOutline': {
            borderWidth: 2,
          },
          '&.Mui-disabled': {
            backgroundColor: alpha(mvforgeTokens.colors.surfaceRaised, 0.48),
          },
          '&.Mui-disabled .MuiOutlinedInput-notchedOutline': {
            borderColor: mvforgeTokens.colors.border.subtle,
          },
          '&.MuiInputBase-sizeSmall': {
            minHeight: mvforgeTokens.density.controlHeight.compact,
          },
          '[data-ui-density="compact"] &': {
            minHeight: mvforgeTokens.density.controlHeight.compact,
          },
          '&.MuiInputBase-sizeSmall .MuiOutlinedInput-input': {
            padding: '5px 10px',
            fontSize: mvforgeTokens.typography.compactBody,
          },
          '[data-ui-density="compact"] & .MuiOutlinedInput-input': {
            padding: '5px 10px',
            fontSize: mvforgeTokens.typography.compactBody,
          },
        },
      },
    },
    MuiInputLabel: {
      styleOverrides: {
        root: {
          color: mvforgeTokens.colors.text.secondary,
          '&.Mui-focused': { color: mvforgeTokens.colors.accent.primary },
          '&.Mui-error': { color: mvforgeTokens.colors.status.error },
          '&.Mui-disabled': { color: mvforgeTokens.colors.text.disabled },
        },
      },
    },
    MuiFormHelperText: {
      styleOverrides: {
        root: {
          color: mvforgeTokens.colors.text.muted,
          fontSize: mvforgeTokens.typography.supporting,
          '&.Mui-error': { color: mvforgeTokens.colors.status.error },
          '&.Mui-disabled': { color: mvforgeTokens.colors.text.disabled },
        },
      },
    },
    MuiSelect: {
      styleOverrides: {
        icon: { color: mvforgeTokens.colors.text.secondary },
        select: {
          '&.Mui-disabled': { WebkitTextFillColor: mvforgeTokens.colors.text.disabled },
        },
      },
    },
    MuiAutocomplete: {
      styleOverrides: {
        inputRoot: {
          '&.MuiInputBase-sizeSmall': {
            minHeight: mvforgeTokens.density.controlHeight.compact,
          },
          '[data-ui-density="compact"] &': {
            minHeight: mvforgeTokens.density.controlHeight.compact,
          },
        },
        paper: {
          backgroundColor: mvforgeTokens.colors.surfaceRaised,
          border: `1px solid ${mvforgeTokens.colors.border.subtle}`,
        },
        option: {
          minHeight: mvforgeTokens.density.controlHeight.compact,
          color: mvforgeTokens.colors.text.primary,
          '&.Mui-focused': { backgroundColor: mvforgeTokens.interaction.hoverBackground },
          '&[aria-selected="true"]': { backgroundColor: mvforgeTokens.colors.surfaceSelected },
          '&[aria-selected="true"].Mui-focused': {
            backgroundColor: mvforgeTokens.interaction.selectedBackground,
          },
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: mvforgeTokens.colors.surface,
          border: `1px solid ${mvforgeTokens.colors.border.subtle}`,
          borderRadius: 8,
          boxShadow: 'none',
        },
      },
    },
    MuiCardActionArea: {
      styleOverrides: {
        root: {
          height: '100%',
          '&:hover': {
            backgroundColor: mvforgeTokens.colors.surfaceInteractive,
          },
          '&.Mui-focusVisible': {
            outline: `2px solid ${mvforgeTokens.interaction.focus}`,
            outlineOffset: 2,
          },
          '&.Mui-selected': { background: mvforgeTokens.interaction.selectedBackground },
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: mvforgeTokens.colors.surface,
          '&.MuiPaper-outlined': { borderColor: mvforgeTokens.colors.border.default },
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: { backgroundColor: mvforgeTokens.colors.surfaceRaised },
      },
    },
    MuiPopover: {
      styleOverrides: {
        paper: {
          backgroundColor: mvforgeTokens.colors.surfaceRaised,
          border: `1px solid ${mvforgeTokens.colors.border.subtle}`,
          borderRadius: 8,
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundImage: 'none',
          backgroundColor: mvforgeTokens.colors.surfaceRaised,
          border: `1px solid ${mvforgeTokens.colors.border.subtle}`,
          borderRadius: 8,
        },
      },
    },
    MuiDialogTitle: {
      styleOverrides: {
        root: {
          color: mvforgeTokens.colors.text.primary,
          fontSize: mvforgeTokens.typography.pageHeading,
          fontWeight: 700,
        },
      },
    },
    MuiDialogContent: {
      styleOverrides: {
        dividers: { borderColor: mvforgeTokens.colors.border.subtle },
      },
    },
    MuiDialogActions: {
      styleOverrides: {
        root: {
          borderTop: `1px solid ${mvforgeTokens.colors.border.subtle}`,
          padding: '8px 16px',
        },
      },
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          borderRadius: 6,
          fontSize: mvforgeTokens.typography.compactBody,
          '&.MuiAlert-standardSuccess': {
            color: mvforgeTokens.colors.status.success,
            backgroundColor: mvforgeTokens.colors.status.successSurface,
          },
          '&.MuiAlert-standardWarning': {
            color: mvforgeTokens.colors.status.warning,
            backgroundColor: mvforgeTokens.colors.status.warningSurface,
          },
          '&.MuiAlert-standardError': {
            color: mvforgeTokens.colors.status.error,
            backgroundColor: mvforgeTokens.colors.status.errorSurface,
          },
          '&.MuiAlert-standardInfo': {
            color: mvforgeTokens.colors.status.info,
            backgroundColor: mvforgeTokens.colors.status.infoSurface,
          },
        },
      },
    },
    MuiToolbar: {
      styleOverrides: {
        root: {
          '&.MuiToolbar-dense': {
            minHeight: mvforgeTokens.density.controlHeight.normal,
            flexWrap: 'wrap',
            gap: 8,
          },
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderColor: mvforgeTokens.colors.border.subtle,
          color: mvforgeTokens.colors.text.primary,
          '&.MuiTableCell-sizeSmall': {
            fontSize: mvforgeTokens.typography.compactBody,
          },
          'tbody tr:not(.Mui-selected) &': {
            backgroundColor: mvforgeTokens.colors.surface,
          },
          'table[data-ui-selection="tinted"] tr.Mui-selected &': {
            backgroundColor: mvforgeTokens.colors.surfaceSelected,
          },
          'tr.MuiTableRow-hover:hover &': {
            backgroundColor: mvforgeTokens.colors.surfaceInteractive,
          },
          'table[data-ui-selection="tinted"] tr.Mui-selected.MuiTableRow-hover:hover &': {
            backgroundColor: mvforgeTokens.colors.surfaceSelected,
          },
        },
        head: {
          color: mvforgeTokens.colors.text.secondary,
          fontWeight: 700,
          backgroundColor: mvforgeTokens.colors.surfaceRaised,
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          '&.MuiTableRow-hover:hover': {
            backgroundColor: mvforgeTokens.colors.surfaceInteractive,
          },
          'table[data-ui-selection="tinted"] &.Mui-selected': {
            backgroundColor: mvforgeTokens.colors.surfaceSelected,
          },
          'table[data-ui-selection="tinted"] &.Mui-selected.MuiTableRow-hover:hover': {
            backgroundColor: mvforgeTokens.colors.surfaceSelected,
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 6,
          border: `1px solid ${mvforgeTokens.colors.border.subtle}`,
          backgroundColor: mvforgeTokens.colors.surfaceRaised,
          color: mvforgeTokens.colors.text.secondary,
          fontWeight: 600,
          '&.MuiChip-sizeSmall': { height: 24 },
          '&.MuiChip-colorPrimary': {
            color: mvforgeTokens.colors.status.info,
            backgroundColor: mvforgeTokens.colors.status.infoSurface,
            borderColor: alpha(mvforgeTokens.colors.status.info, 0.28),
          },
          '&.MuiChip-colorSecondary': {
            color: mvforgeTokens.colors.accent.secondary,
            backgroundColor: mvforgeTokens.colors.accent.secondarySurface,
            borderColor: alpha(mvforgeTokens.colors.accent.secondary, 0.28),
          },
          '&.MuiChip-colorSuccess': {
            color: mvforgeTokens.colors.status.success,
            backgroundColor: mvforgeTokens.colors.status.successSurface,
            borderColor: alpha(mvforgeTokens.colors.status.success, 0.28),
          },
          '&.MuiChip-colorWarning': {
            color: mvforgeTokens.colors.status.warning,
            backgroundColor: mvforgeTokens.colors.status.warningSurface,
            borderColor: alpha(mvforgeTokens.colors.status.warning, 0.28),
          },
          '&.MuiChip-colorError': {
            color: mvforgeTokens.colors.status.error,
            backgroundColor: mvforgeTokens.colors.status.errorSurface,
            borderColor: alpha(mvforgeTokens.colors.status.error, 0.28),
          },
          '&.MuiChip-colorInfo': {
            color: mvforgeTokens.colors.status.info,
            backgroundColor: mvforgeTokens.colors.status.infoSurface,
            borderColor: alpha(mvforgeTokens.colors.status.info, 0.28),
          },
          '&.MuiChip-outlined': { backgroundColor: 'transparent' },
        },
      },
    },
  },
});
