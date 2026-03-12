import { useState, useCallback } from 'react';
import styles from './CommandDrawer.module.css';

interface CommandDrawerProps {
  open: boolean;
  onClose: () => void;
  hapticTap?: () => void;
}

interface QuickCommand {
  label: string;
  /** Command text sent to terminal. Ignored if `action` is set. */
  cmd: string;
  /** If true, press Enter after pasting */
  run?: boolean;
  /** Raw bytes to send instead of `cmd` (e.g. control chars) */
  action?: string;
  /** Optional short description shown below the label */
  desc?: string;
}

interface CommandGroup {
  title: string;
  icon: string;
  commands: QuickCommand[];
}

const COMMAND_GROUPS: CommandGroup[] = [
  {
    title: 'AI',
    icon: '\u2728', // sparkles
    commands: [
      { label: 'claude', cmd: 'claude', run: true, desc: 'Start AI assistant' },
      { label: 'claude login', cmd: 'claude login', run: true, desc: 'Authenticate CLI' },
      { label: '/compact', cmd: '/compact', run: true, desc: 'Compact context' },
      { label: '/clear', cmd: '/clear', run: true, desc: 'Clear conversation' },
      { label: '/diff', cmd: '/diff', run: true, desc: 'Show pending diff' },
      { label: '/cost', cmd: '/cost', run: true, desc: 'Token usage' },
      { label: '/help', cmd: '/help', run: true, desc: 'Show commands' },
      { label: '/model', cmd: '/model ', run: false, desc: 'Switch model' },
      { label: 'Use Claude', cmd: '/model claude-sonnet-4-20250514', run: true, desc: 'OpenRouter: Claude' },
      { label: 'Use GPT-4o', cmd: '/model openai/gpt-4o', run: true, desc: 'OpenRouter: GPT-4o' },
      { label: 'Use Gemini', cmd: '/model google/gemini-2.5-pro-preview-06-05', run: true, desc: 'OpenRouter: Gemini' },
      { label: 'Use Llama', cmd: '/model meta-llama/llama-4-maverick', run: true, desc: 'OpenRouter: Llama' },
    ],
  },
  {
    title: 'Memory',
    icon: '\uD83E\uDDE0', // brain
    commands: [
      { label: 'same status', cmd: 'same status', run: true, desc: 'Check vault status' },
      { label: 'same search', cmd: 'same search ', run: false, desc: 'Search memory (type query)' },
      { label: 'same log', cmd: 'same log', run: true, desc: 'Recent activity' },
      { label: 'same graph', cmd: 'same graph', run: true, desc: 'Show knowledge graph' },
      { label: 'same ingest', cmd: 'same ingest ', run: false, desc: 'Ingest a file' },
    ],
  },
  {
    title: 'Shell',
    icon: '\u25B6', // play triangle
    commands: [
      { label: 'ls', cmd: 'ls -la', run: true, desc: 'List files' },
      { label: 'clear', cmd: '', run: false, action: '\x0c', desc: 'Clear terminal' },
      { label: 'exit', cmd: 'exit', run: true, desc: 'Exit session' },
      { label: 'pwd', cmd: 'pwd', run: true, desc: 'Print directory' },
      { label: 'cd ..', cmd: 'cd ..', run: true, desc: 'Go up one level' },
      { label: 'history', cmd: 'history', run: true, desc: 'Command history' },
    ],
  },
  {
    title: 'Files',
    icon: '\uD83D\uDCC2', // open folder
    commands: [
      { label: 'tree', cmd: 'tree -L 2', run: true, desc: 'Directory tree' },
      { label: 'cat', cmd: 'cat ', run: false, desc: 'View file contents' },
      { label: 'mkdir', cmd: 'mkdir ', run: false, desc: 'Create directory' },
      { label: 'find', cmd: 'find . -name ', run: false, desc: 'Find files by name' },
    ],
  },
  {
    title: 'Git',
    icon: '\uD83D\uDD00', // shuffle (branch-like)
    commands: [
      { label: 'status', cmd: 'git status', run: true, desc: 'Working tree status' },
      { label: 'log', cmd: 'git log --oneline -10', run: true, desc: 'Recent commits' },
      { label: 'diff', cmd: 'git diff', run: true, desc: 'Unstaged changes' },
      { label: 'add .', cmd: 'git add .', run: true, desc: 'Stage all files' },
      { label: 'commit', cmd: 'git commit -m "', run: false, desc: 'Commit with message' },
      { label: 'push', cmd: 'git push', run: true, desc: 'Push to remote' },
      { label: 'pull', cmd: 'git pull', run: true, desc: 'Pull from remote' },
      { label: 'branch', cmd: 'git branch', run: true, desc: 'List branches' },
    ],
  },
];

function getWsSend(): ((data: string) => void) | null {
  return (window as unknown as Record<string, unknown>).__wsSend as ((data: string) => void) | null;
}

export default function CommandDrawer({ open, onClose, hapticTap }: CommandDrawerProps) {
  const [activeGroup, setActiveGroup] = useState(0);

  const runCommand = useCallback((cmd: QuickCommand) => {
    hapticTap?.();
    const send = getWsSend();
    if (!send) return;

    if (cmd.action) {
      // Send raw action bytes (e.g. Ctrl+L for clear)
      send(cmd.action);
    } else {
      // Send command text directly to WebSocket
      send(cmd.cmd);

      // If auto-run, send Enter
      if (cmd.run) {
        setTimeout(() => send('\r'), 30);
      }
    }

    onClose();
  }, [hapticTap, onClose]);

  if (!open) return null;

  return (
    <div className={styles.overlay} onPointerDown={onClose}>
      <div className={styles.drawer} onPointerDown={(e) => e.stopPropagation()}>
        <div className={styles.handle} />

        <div className={styles.tabs}>
          {COMMAND_GROUPS.map((group, i) => (
            <button
              key={group.title}
              className={`${styles.tab} ${i === activeGroup ? styles.tabActive : ''}`}
              onPointerDown={(e) => {
                e.preventDefault();
                hapticTap?.();
                setActiveGroup(i);
              }}
            >
              <span className={styles.tabIcon}>{group.icon}</span>
              {group.title}
            </button>
          ))}
        </div>

        <div className={styles.commands}>
          {COMMAND_GROUPS[activeGroup].commands.map((cmd) => (
            <button
              key={cmd.label}
              className={styles.cmdBtn}
              onPointerDown={(e) => {
                e.preventDefault();
                runCommand(cmd);
              }}
            >
              <div className={styles.cmdInfo}>
                <span className={styles.cmdLabel}>{cmd.label}</span>
                {cmd.desc && <span className={styles.cmdDesc}>{cmd.desc}</span>}
              </div>
              {cmd.run ? (
                <span className={styles.cmdRun}>run</span>
              ) : !cmd.action ? (
                <span className={styles.cmdEdit}>edit</span>
              ) : (
                <span className={styles.cmdRun}>send</span>
              )}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
