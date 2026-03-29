import { useContext } from 'react';
import { ThemeContext } from '@/theme/ThemeContextShared';

export const useThemeControl = () => useContext(ThemeContext);
