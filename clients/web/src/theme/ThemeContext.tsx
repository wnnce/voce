import React, { useEffect, useState } from 'react';
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
    if (mode === 'dark') {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }
    const tokens = mode === 'light' ? lightTokens : darkTokens;
    Object.entries(tokens).forEach(([key, value]) => {
      document.documentElement.style.setProperty(key, value as string);
    });
  }, [mode]);

  return (
    <ThemeContext.Provider value={{ mode, toggleTheme }}>
      {children}
    </ThemeContext.Provider>
  );
};
