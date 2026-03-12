import { useState, useEffect, useCallback, useRef } from 'react';
import styles from './QuickReply.module.css';

/** Get the global WebSocket send function exposed by HtmlTerminal */
function getWsSend(): ((data: string) => void) | null {
  return (window as unknown as Record<string, unknown>).__wsSend as ((data: string) => void) | null;
}

/** Detected option for quick reply */
export interface QuickOption {
  /** The value to send (e.g. "1", "y", "\r") */
  value: string;
  /** Display label */
  label: string;
  /** Type of option */
  type: 'numbered' | 'yesno' | 'continue';
}

/** Custom event detail shape */
export interface QuickReplyEventDetail {
  options: QuickOption[];
}

/** Event name used for communication between HtmlTerminal and QuickReply */
export const QUICK_REPLY_EVENT = 'quick-reply-options';

/**
 * Scan terminal lines for numbered options, yes/no prompts, or "press Enter" prompts.
 * Returns detected options or null if nothing found.
 */
export function detectOptions(lines: string[]): QuickOption[] | null {
  // Scan from the bottom up (most recent output)
  // We look at the last 20 lines

  const tail = lines.slice(-20);

  // --- 1. "Press Enter to continue" detection ---
  for (let i = tail.length - 1; i >= Math.max(0, tail.length - 5); i--) {
    const line = tail[i].toLowerCase();
    if (
      line.includes('press enter') ||
      line.includes('press return') ||
      line.includes('hit enter') ||
      line.includes('press any key')
    ) {
      return [{ value: '\r', label: 'Continue', type: 'continue' }];
    }
  }

  // --- 2. Yes/No prompt detection ---
  // Look at the last 3 lines for y/n prompts
  for (let i = tail.length - 1; i >= Math.max(0, tail.length - 3); i--) {
    const line = tail[i];
    // Match patterns like (y/n), [y/N], [Y/n], (yes/no), [yes/no]
    if (/\(y\/n\)|\[y\/n\]|\[Y\/n\]|\[y\/N\]|\(yes\/no\)|\[yes\/no\]/i.test(line)) {
      // Determine default from capitalization
      const hasDefaultYes = /\[Y\/n\]/i.test(line) && !/\[y\/N\]/.test(line);
      const hasDefaultNo = /\[y\/N\]/.test(line);

      const yesLabel = hasDefaultYes ? 'Yes (default)' : 'Yes';
      const noLabel = hasDefaultNo ? 'No (default)' : 'No';

      return [
        { value: 'y\r', label: yesLabel, type: 'yesno' },
        { value: 'n\r', label: noLabel, type: 'yesno' },
      ];
    }
  }

  // --- 3. Numbered options detection ---
  // Find consecutive numbered lines (1. / 2. / 3. or 1) / 2) / 3))
  // We need at least 2 consecutive numbers starting from 1.

  // Build plain text from tail lines and search for numbered patterns
  const numberedOptions: QuickOption[] = [];
  let foundStart = -1;
  let expectedNext = 1;
  // Track which separator style we're using (dot or paren)
  let separatorStyle: 'dot' | 'paren' | null = null;

  for (let i = 0; i < tail.length; i++) {
    const line = tail[i];

    // Match: optional whitespace, then number, then . or ), then space, then label text
    const dotMatch = line.match(/^\s*(\d+)\.\s+(.+)/);
    const parenMatch = line.match(/^\s*(\d+)\)\s+(.+)/);

    const match = dotMatch || parenMatch;
    const currentSep = dotMatch ? 'dot' : parenMatch ? 'paren' : null;

    if (match && currentSep) {
      const num = parseInt(match[1], 10);

      if (num === expectedNext && (separatorStyle === null || separatorStyle === currentSep)) {
        if (num === 1) {
          foundStart = i;
          separatorStyle = currentSep;
          numberedOptions.length = 0; // Reset
        }

        // Extract label: first ~30 chars, trimmed
        let label = match[2].trim();
        if (label.length > 30) {
          label = label.slice(0, 28) + '...';
        }

        numberedOptions.push({
          value: String(num) + '\r',
          label,
          type: 'numbered',
        });
        expectedNext = num + 1;
      } else if (num === 1) {
        // Restart detection (new numbered list started)
        foundStart = i;
        separatorStyle = currentSep;
        expectedNext = 2;
        numberedOptions.length = 0;

        let label = match[2].trim();
        if (label.length > 30) {
          label = label.slice(0, 28) + '...';
        }

        numberedOptions.push({
          value: '1\r',
          label,
          type: 'numbered',
        });
      } else {
        // Non-consecutive number — reset
        foundStart = -1;
        expectedNext = 1;
        separatorStyle = null;
        numberedOptions.length = 0;
      }
    } else if (foundStart >= 0) {
      // Non-matching line after we started finding numbers
      // Allow a blank line or short non-numbered line between options
      // But if we get more than 2 non-matching lines in a row, reset
      const trimmed = line.trim();
      if (trimmed.length > 0 && !trimmed.startsWith('─') && !trimmed.startsWith('—')) {
        // Check if the NEXT line continues the numbering
        if (i + 1 < tail.length) {
          const nextLine = tail[i + 1];
          const nextDot = nextLine.match(/^\s*(\d+)\.\s+(.+)/);
          const nextParen = nextLine.match(/^\s*(\d+)\)\s+(.+)/);
          const nextMatch = nextDot || nextParen;
          if (nextMatch && parseInt(nextMatch[1], 10) === expectedNext) {
            // There's a continuation, allow this gap
            continue;
          }
        }
        // If we already have 2+ options, stop collecting (the list ended)
        if (numberedOptions.length >= 2) {
          break;
        }
        // Otherwise, reset
        foundStart = -1;
        expectedNext = 1;
        separatorStyle = null;
        numberedOptions.length = 0;
      }
    }
  }

  // Only return numbered options if we found at least 2 consecutive
  if (numberedOptions.length >= 2) {
    return numberedOptions;
  }

  return null;
}

interface QuickReplyProps {
  hapticTap?: () => void;
}

/**
 * QuickReply — floating quick-reply buttons above the InputBar.
 *
 * Listens for 'quick-reply-options' custom events dispatched by HtmlTerminal
 * when it detects numbered options, yes/no prompts, or "press Enter" prompts
 * in the recent terminal output.
 *
 * Tapping a button sends the corresponding value directly to the terminal
 * via __wsSend.
 */
export default function QuickReply({ hapticTap }: QuickReplyProps) {
  const [options, setOptions] = useState<QuickOption[] | null>(null);
  const staleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Listen for option detection events
  useEffect(() => {
    const handleOptions = (e: Event) => {
      const detail = (e as CustomEvent<QuickReplyEventDetail>).detail;
      if (detail.options && detail.options.length > 0) {
        setOptions(detail.options);

        // Clear any existing stale timer
        if (staleTimerRef.current) {
          clearTimeout(staleTimerRef.current);
        }

        // Auto-hide after 30 seconds
        staleTimerRef.current = setTimeout(() => {
          setOptions(null);
        }, 30000);
      } else {
        setOptions(null);
      }
    };

    const handleClear = () => {
      setOptions(null);
      if (staleTimerRef.current) {
        clearTimeout(staleTimerRef.current);
        staleTimerRef.current = null;
      }
    };

    window.addEventListener(QUICK_REPLY_EVENT, handleOptions);
    window.addEventListener('quick-reply-clear', handleClear);

    return () => {
      window.removeEventListener(QUICK_REPLY_EVENT, handleOptions);
      window.removeEventListener('quick-reply-clear', handleClear);
      if (staleTimerRef.current) {
        clearTimeout(staleTimerRef.current);
      }
    };
  }, []);

  const handleTap = useCallback((option: QuickOption) => {
    hapticTap?.();
    const send = getWsSend();
    if (send) {
      send(option.value);
    }
    // Hide after sending
    setOptions(null);
    if (staleTimerRef.current) {
      clearTimeout(staleTimerRef.current);
      staleTimerRef.current = null;
    }
  }, [hapticTap]);

  if (!options || options.length === 0) {
    return null;
  }

  return (
    <div className={styles.container}>
      <div className={styles.bar}>
        {options.map((opt, i) => {
          const isSpecial = opt.type === 'yesno' || opt.type === 'continue';
          return (
            <button
              key={i}
              className={isSpecial ? styles.pillSpecial : styles.pill}
              onPointerDown={(e) => {
                e.preventDefault();
                handleTap(opt);
              }}
              onContextMenu={(e) => e.preventDefault()}
            >
              {opt.type === 'numbered' && (
                <span className={styles.pillNumber}>{opt.value.replace('\r', '')}.</span>
              )}
              <span className={styles.pillLabel}>{opt.label}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}
