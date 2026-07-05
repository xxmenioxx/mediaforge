import { createTheme } from '@mui/material/styles';

export const theme = createTheme({
  palette: {
    mode: 'dark',
    background: {
      default: '#0e1116',
      paper: '#171b22',
    },
    primary: {
      main: '#4fb3ff',
    },
    secondary: {
      main: '#66d9a8',
    },
    warning: {
      main: '#f6b44b',
    },
    divider: 'rgba(255,255,255,0.08)',
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
          backgroundColor: '#171b22',
          border: '1px solid rgba(255,255,255,0.08)',
          boxShadow: 'none',
        },
      },
    },
    MuiCardActionArea: {
      styleOverrides: {
        root: {
          height: '100%',
          '&:hover': {
            backgroundColor: 'rgba(79,179,255,0.05)',
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
        containedPrimary: {
          color: '#07121d',
          backgroundColor: '#4fb3ff',
          boxShadow: 'none',
          '&:hover': {
            backgroundColor: '#74c5ff',
            boxShadow: 'none',
          },
          '&.Mui-disabled': {
            backgroundColor: 'rgba(79,179,255,0.22)',
          },
        },
        outlinedPrimary: {
          borderColor: 'rgba(79,179,255,0.55)',
          color: '#4fb3ff',
          '&:hover': {
            borderColor: '#4fb3ff',
            backgroundColor: 'rgba(79,179,255,0.08)',
          },
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          backgroundImage: 'none',
          backgroundColor: '#171b22',
          border: '1px solid rgba(255,255,255,0.1)',
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderColor: 'rgba(255,255,255,0.08)',
        },
        head: {
          color: 'rgba(255,255,255,0.72)',
          fontWeight: 700,
          backgroundColor: 'rgba(255,255,255,0.02)',
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          '&.MuiTableRow-hover:hover': {
            backgroundColor: 'rgba(79,179,255,0.045)',
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
