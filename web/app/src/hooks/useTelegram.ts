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

interface SafeAreaInset {
  top: number;
  bottom: number;
  left: number;
  right: number;
}

interface TelegramMainButton {
  text: string;
  isVisible: boolean;
  show: () => void;
  hide: () => void;
  onClick: (callback: () => void) => void;
  offClick: (callback: () => void) => void;
  setText: (text: string) => void;
  setParams: (params: { text?: string; color?: string; text_color?: string; is_active?: boolean; is_visible?: boolean }) => void;
}

interface TelegramWebApp {
  ready: () => void;
  expand: () => void;
  requestFullscreen?: () => void;
  exitFullscreen?: () => void;
  disableVerticalSwipes?: () => void;
  themeParams: TelegramTheme;
  colorScheme: 'light' | 'dark';
  viewportHeight?: number;
  viewportStableHeight?: number;
  isFullscreen?: boolean;
  safeAreaInset?: SafeAreaInset;
  contentSafeAreaInset?: SafeAreaInset;
  MainButton?: TelegramMainButton;
  HapticFeedback: {
    impactOccurred: (style: 'light' | 'medium' | 'heavy' | 'rigid' | 'soft') => void;
    notificationOccurred: (type: 'error' | 'success' | 'warning') => void;
    selectionChanged: () => void;
  };
  openLink?: (url: string) => void;
  readTextFromClipboard?: (callback: (text: string | null) => void) => void;
  hideKeyboard?: () => void;
  onEvent: (event: string, callback: (...args: unknown[]) => void) => void;
  offEvent: (event: string, callback: (...args: unknown[]) => void) => void;
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

    // Don't use requestFullscreen() — it pushes content behind the
    // Telegram header (Close/minimize buttons) and contentSafeAreaInset
    // isn't reliably supported across Telegram versions. expand() alone
    // gives us the full area below the native header.

    // Disable vertical swipe-to-close while in terminal
    if (tg.disableVerticalSwipes) {
      tg.disableVerticalSwipes();
    }

    // Apply theme CSS variables
    applyThemeVars(tg.themeParams);
    applySafeAreaVars(tg);

    // Track Telegram viewport height (changes with keyboard open/close).
    // This CSS variable is used by App.tsx for accurate layout height.
    const syncViewportHeight = () => {
      if (tg.viewportStableHeight) {
        document.documentElement.style.setProperty(
          '--tg-viewport-height',
          `${tg.viewportStableHeight}px`
        );
      } else if (tg.viewportHeight) {
        document.documentElement.style.setProperty(
          '--tg-viewport-height',
          `${tg.viewportHeight}px`
        );
      }
    };
    syncViewportHeight();

    // Listen for viewport changes (keyboard open/close)
    const handleViewportChange = () => {
      syncViewportHeight();
    };

    // Listen for theme changes (user switches dark/light mode)
    const handleThemeChange = () => {
      if (tg.themeParams) {
        applyThemeVars(tg.themeParams);
        setColorScheme(tg.colorScheme || 'dark');
      }
    };

    // Re-sync safe area on fullscreen/viewport changes
    const handleSafeAreaChange = () => applySafeAreaVars(tg);

    tg.onEvent('themeChanged', handleThemeChange);
    tg.onEvent('viewportChanged', handleViewportChange);
    tg.onEvent('fullscreenChanged', handleSafeAreaChange);
    tg.onEvent('safeAreaChanged', handleSafeAreaChange);
    tg.onEvent('contentSafeAreaChanged', handleSafeAreaChange);

    return () => {
      tg.offEvent('themeChanged', handleThemeChange);
      tg.offEvent('viewportChanged', handleViewportChange);
      tg.offEvent('fullscreenChanged', handleSafeAreaChange);
      tg.offEvent('safeAreaChanged', handleSafeAreaChange);
      tg.offEvent('contentSafeAreaChanged', handleSafeAreaChange);

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
 * Sync Telegram safe area insets to CSS custom properties.
 * In fullscreen mode, contentSafeAreaInset provides the offset
 * needed to clear the Telegram header / Dynamic Island.
 */
function applySafeAreaVars(tg: TelegramWebApp) {
  const root = document.documentElement;
  const sa = tg.safeAreaInset;
  const csa = tg.contentSafeAreaInset;

  if (sa) {
    root.style.setProperty('--tg-safe-top', `${sa.top}px`);
    root.style.setProperty('--tg-safe-bottom', `${sa.bottom}px`);
    root.style.setProperty('--tg-safe-left', `${sa.left}px`);
    root.style.setProperty('--tg-safe-right', `${sa.right}px`);
  }
  if (csa) {
    root.style.setProperty('--tg-content-safe-top', `${csa.top}px`);
    root.style.setProperty('--tg-content-safe-bottom', `${csa.bottom}px`);
  }
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
