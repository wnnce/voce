import React, { useEffect, useState, useMemo } from 'react';
import { createTheme, ThemeProvider, CssBaseline } from '@mui/material';
import { lightTokens, darkTokens } from '@/theme/tokens';
import { ThemeContext, type ThemeMode } from '@/theme/ThemeContextShared';

export { ThemeContext, type ThemeMode };

export const CustomThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [mode, setMode] = useState<ThemeMode>(() => {
    const saved = localStorage.getItem('theme-mode');
    return (saved as ThemeMode) || 'dark';
  });

  const toggleTheme = () => {
    setMode((prev) => (prev === 'light' ? 'dark' : 'light'));
  };

  useEffect(() => {
    localStorage.setItem('theme-mode', mode);
    const tokens = mode === 'light' ? lightTokens : darkTokens;
    Object.entries(tokens).forEach(([key, value]) => {
      document.documentElement.style.setProperty(key, value as string);
    });
  }, [mode]);

  const theme = useMemo(
    () =>
      createTheme({
        palette: {
          mode,
          primary: {
            main: mode === 'light' ? lightTokens['--primary-color'] : darkTokens['--primary-color'],
          },
          background: {
            default: mode === 'light' ? lightTokens['--bg-color'] : darkTokens['--bg-color'],
            paper: mode === 'light' ? lightTokens['--bg-paper'] : darkTokens['--bg-paper'],
          },
          text: {
            primary: mode === 'light' ? lightTokens['--text-primary'] : darkTokens['--text-primary'],
            secondary: mode === 'light' ? lightTokens['--text-secondary'] : darkTokens['--text-secondary'],
          },
          divider: mode === 'light' ? lightTokens['--border-color'] : darkTokens['--border-color'],
        },
        typography: {
          fontFamily: '"Outfit", "Inter", "Roboto", "Helvetica", "Arial", sans-serif',
        },
        components: {
          MuiButton: {
            styleOverrides: {
              root: {
                textTransform: 'none',
                borderRadius: 10,
                boxShadow: 'none',
                fontWeight: 500,
                '&:hover': {
                  boxShadow: 'none',
                },
                '&.MuiButton-text:hover, &.MuiButton-outlined:hover': {
                  backgroundColor: mode === 'light' ? 'rgba(0,122,255,0.06)' : 'rgba(10,132,255,0.1)',
                },
                '&.MuiButton-contained': {
                   boxShadow: 'none',
                   '&:hover': {
                     boxShadow: 'none',
                     // MUI will handle the darker shade naturally if we don't override it with a light transparent one
                   }
                }
              },
            },
          },
          MuiAppBar: {
            styleOverrides: {
              root: {
                boxShadow: 'none',
                borderBottom: '1px solid var(--border-color)',
                backgroundColor: 'var(--bg-paper)',
                color: 'var(--text-primary)',
                borderRadius: 0,
              }
            }
          },
          MuiDrawer: {
            styleOverrides: {
              paper: {
                borderRadius: 0,
                border: 'none',
                borderRight: '1px solid var(--border-color)',
              }
            }
          },
          MuiPaper: {
            styleOverrides: {
              root: {
                backgroundImage: 'none',
                borderRadius: 14,
                boxShadow: mode === 'light' ? '0 1px 3px rgba(0,0,0,0.03)' : '0 1px 3px rgba(0,0,0,0.3)',
                borderStyle: 'solid',
                borderWidth: '1px',
                borderColor: 'var(--border-color)',
              },
            },
          },
        },
      }),
    [mode]
  );

  return (
    <ThemeContext.Provider value={{ mode, toggleTheme }}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        {children}
      </ThemeProvider>
    </ThemeContext.Provider>
  );
};
