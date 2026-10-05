import { alpha, createTheme } from '@mui/material/styles';

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
    MuiCard: {
      styleOverrides: {
        root: {
          backgroundImage: 'none',
          backgroundColor: mvforgeTokens.colors.surface,
          border: `1px solid ${mvforgeTokens.colors.border.subtle}`,
          boxShadow: 'none',
        },
      },
    },
    MuiCardActionArea: {
      styleOverrides: {
        root: {
          height: '100%',
          '&:hover': {
            backgroundColor: mvforgeTokens.interaction.hoverBackground,
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
        },
        containedPrimary: ({ theme: muiTheme }) => ({
          color: mvforgeTokens.colors.canvas,
          backgroundColor: muiTheme.palette.primary.main,
          boxShadow: 'none',
          '&:hover': {
            filter: 'brightness(1.1)',
            boxShadow: 'none',
          },
          '&.Mui-disabled': {
            backgroundColor: alpha(muiTheme.palette.primary.main, 0.22),
          },
        }),
        outlinedPrimary: ({ theme: muiTheme }) => ({
          borderColor: alpha(muiTheme.palette.primary.main, 0.55),
          color: muiTheme.palette.primary.main,
          '&:hover': {
            borderColor: muiTheme.palette.primary.main,
            backgroundColor: alpha(muiTheme.palette.primary.main, 0.08),
          },
        }),
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundImage: 'none',
          backgroundColor: mvforgeTokens.colors.surface,
          border: `1px solid ${mvforgeTokens.colors.border.subtle}`,
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderColor: mvforgeTokens.colors.border.subtle,
        },
        head: {
          color: mvforgeTokens.colors.text.secondary,
          fontWeight: 700,
          backgroundColor: 'rgba(255,255,255,0.02)',
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          '&.MuiTableRow-hover:hover': {
            backgroundColor: mvforgeTokens.interaction.hoverBackground,
          },
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          fontWeight: 700,
        },
      },
    },
  },
});
