import { useEffect, useRef, useState } from 'react';

/**
 * Telegram WebApp SDK integration.
 *
 * Handles initialization, theme sync, haptic feedback,
 * fullscreen mode, and swipe behavior.
 */

interface TelegramTheme {
  bg_color?: string;
  secondary_bg_color?: string;
  text_color?: string;
  hint_color?: string;
  link_color?: string;
  button_color?: string;
  button_text_color?: string;
  accent_text_color?: string;
  section_bg_color?: string;
  section_header_text_color?: string;
  section_separator_color?: string;
  subtitle_text_color?: string;
  destructive_text_color?: string;
  header_bg_color?: string;
}

interface TelegramWebApp {
  ready: () => void;
  expand: () => void;
  requestFullscreen?: () => void;
  disableVerticalSwipes?: () => void;
  themeParams: TelegramTheme;
  colorScheme: 'light' | 'dark';
  isFullscreen?: boolean;
  HapticFeedback: {
    impactOccurred: (style: 'light' | 'medium' | 'heavy' | 'rigid' | 'soft') => void;
    notificationOccurred: (type: 'error' | 'success' | 'warning') => void;
    selectionChanged: () => void;
  };
  onEvent: (event: string, callback: () => void) => void;
  offEvent: (event: string, callback: () => void) => void;
}

declare global {
  interface Window {
    Telegram?: {
      WebApp: TelegramWebApp;
    };
  }
}

export function useTelegram() {
  const [isInTelegram, setIsInTelegram] = useState(false);
  const [colorScheme, setColorScheme] = useState<'light' | 'dark'>('dark');
  const webAppRef = useRef<TelegramWebApp | null>(null);

  useEffect(() => {
    const tg = window.Telegram?.WebApp;
    if (!tg) return;

    webAppRef.current = tg;
    setIsInTelegram(true);
    setColorScheme(tg.colorScheme || 'dark');

    // Signal we're ready (hides Telegram's loading placeholder)
    tg.ready();

    // Expand to full height
    tg.expand();

    // Request fullscreen for maximum terminal space
    if (tg.requestFullscreen) {
      try {
        tg.requestFullscreen();
      } catch {
        // Fullscreen not supported in this version
      }
    }

    // Disable vertical swipe-to-close while in terminal
    if (tg.disableVerticalSwipes) {
      tg.disableVerticalSwipes();
    }

    // Apply theme CSS variables
    applyThemeVars(tg.themeParams);

    // Listen for theme changes (user switches dark/light mode)
    const handleThemeChange = () => {
      if (tg.themeParams) {
        applyThemeVars(tg.themeParams);
        setColorScheme(tg.colorScheme || 'dark');
      }
    };

    tg.onEvent('themeChanged', handleThemeChange);

    return () => {
      tg.offEvent('themeChanged', handleThemeChange);
    };
  }, []);

  const haptic = {
    /** Light tap feedback — use for key bar presses */
    tap: () => webAppRef.current?.HapticFeedback.impactOccurred('light'),
    /** Medium impact — use for important actions */
    impact: () => webAppRef.current?.HapticFeedback.impactOccurred('medium'),
    /** Selection change — use for Ctrl/Alt toggle */
    selection: () => webAppRef.current?.HapticFeedback.selectionChanged(),
    /** Success notification — connection established, command succeeded */
    success: () => webAppRef.current?.HapticFeedback.notificationOccurred('success'),
    /** Warning notification */
    warning: () => webAppRef.current?.HapticFeedback.notificationOccurred('warning'),
    /** Error notification — connection lost, command failed */
    error: () => webAppRef.current?.HapticFeedback.notificationOccurred('error'),
  };

  return { isInTelegram, colorScheme, haptic };
}

/**
 * Apply Telegram theme parameters as CSS custom properties.
 * Maps themeParams keys to --tg-theme-* variables.
 */
function applyThemeVars(params: TelegramTheme) {
  const root = document.documentElement;
  const mapping: Record<string, string> = {
    bg_color: '--tg-theme-bg-color',
    secondary_bg_color: '--tg-theme-secondary-bg-color',
    text_color: '--tg-theme-text-color',
    hint_color: '--tg-theme-hint-color',
    link_color: '--tg-theme-link-color',
    button_color: '--tg-theme-button-color',
    button_text_color: '--tg-theme-button-text-color',
    accent_text_color: '--tg-theme-accent-text-color',
    section_bg_color: '--tg-theme-section-bg-color',
    section_header_text_color: '--tg-theme-section-header-text-color',
    section_separator_color: '--tg-theme-section-separator-color',
    subtitle_text_color: '--tg-theme-subtitle-text-color',
    destructive_text_color: '--tg-theme-destructive-text-color',
    header_bg_color: '--tg-theme-header-bg-color',
  };

  for (const [key, cssVar] of Object.entries(mapping)) {
    const value = params[key as keyof TelegramTheme];
    if (value) {
      root.style.setProperty(cssVar, value);
    }
  }
}
