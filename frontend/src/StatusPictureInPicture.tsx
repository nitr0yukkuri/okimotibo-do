import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { moodColor, moodLabel, moodOptions, type Mood } from "./mood";

const pipWidth = 280;
const pipCollapsedHeight = 62;
const pipExpandedHeight = 288;

type DocumentPictureInPictureWindow = {
  requestWindow: (options?: { width?: number; height?: number }) => Promise<Window>;
  window?: Window | null;
};

declare global {
  interface Window {
    documentPictureInPicture?: DocumentPictureInPictureWindow;
  }
}

interface MiniStatusPanelProps {
  mood: Mood;
  onMoodChange: (mood: Mood) => void;
  onExpandedChange?: (expanded: boolean) => void;
  variant?: "launcher" | "pip";
}

function MiniStatusPanel({ mood, onMoodChange, onExpandedChange, variant = "launcher" }: MiniStatusPanelProps) {
  const [expanded, setExpanded] = useState(false);
  const isLauncher = variant === "launcher";
  const updateExpanded = (nextExpanded: boolean) => {
    setExpanded(nextExpanded);
    onExpandedChange?.(nextExpanded);
  };

  return (
    <div className={`mini-status ${expanded ? "is-expanded" : ""} ${isLauncher ? "is-launcher" : ""}`}>
      {expanded ? (
        <>
          <button
            className="mini-arrow mini-arrow-up"
            type="button"
            aria-label="ミニ表示に戻す"
            onClick={() => updateExpanded(false)}
          />
          <div className="mini-mood-controls" role="group" aria-label="会話したい気持ち">
            {moodOptions.map((option) => (
              <button
                className={`mini-mood-button mood-${option.id}`}
                type="button"
                aria-pressed={mood === option.id}
                key={option.id}
                style={{ backgroundColor: moodColor[option.id] }}
                onClick={() => onMoodChange(option.id)}
              >
                {option.label}
              </button>
            ))}
          </div>
          <div
            className="mini-status-card"
            aria-label={`現在の気持ち: ${moodLabel[mood]}`}
            style={{ backgroundColor: moodColor[mood] }}
          />
        </>
      ) : (
        <button
          className="mini-collapsed"
          type="button"
          aria-label={isLauncher ? "常時表示ステータスを開く" : "ステータス操作を開く"}
          onClick={() => updateExpanded(true)}
        >
          <span className="mini-arrow mini-arrow-right" aria-hidden="true" />
          <span className="mini-dot" style={{ backgroundColor: moodColor[mood] }} />
        </button>
      )}
    </div>
  );
}

interface StatusPictureInPictureProps {
  mood: Mood;
  onMoodChange: (mood: Mood) => void;
}

export function StatusPictureInPicture({ mood, onMoodChange }: StatusPictureInPictureProps) {
  const [pipWindow, setPipWindow] = useState<Window | null>(null);
  const [isSupported] = useState(() => Boolean(window.documentPictureInPicture));

  useEffect(() => {
    if (!pipWindow) {
      return;
    }

    const handleClose = () => setPipWindow(null);
    pipWindow.addEventListener("pagehide", handleClose);
    return () => pipWindow.removeEventListener("pagehide", handleClose);
  }, [pipWindow]);

  const openPictureInPicture = async () => {
    if (!window.documentPictureInPicture) {
      return;
    }

    const nextWindow =
      window.documentPictureInPicture.window ??
      (await window.documentPictureInPicture.requestWindow({ width: pipWidth, height: pipCollapsedHeight }));

    nextWindow.document.body.innerHTML = "";
    nextWindow.document.title = "おきもちミニ表示";

    const baseStyle = nextWindow.document.createElement("style");
    baseStyle.textContent = `
      html, body, #pip-root {
        width: 100%;
        min-width: 0;
        height: 100%;
        margin: 0;
        overflow: hidden;
      }
    `;
    nextWindow.document.head.appendChild(baseStyle);

    Array.from(document.styleSheets).forEach((styleSheet) => {
      try {
        const style = nextWindow.document.createElement("style");
        style.textContent = Array.from(styleSheet.cssRules)
          .map((rule) => rule.cssText)
          .join("\n");
        nextWindow.document.head.appendChild(style);
      } catch {
        if (!styleSheet.href) {
          return;
        }

        const link = nextWindow.document.createElement("link");
        link.rel = "stylesheet";
        link.href = styleSheet.href;
        nextWindow.document.head.appendChild(link);
      }
    });

    const root = nextWindow.document.createElement("div");
    root.id = "pip-root";
    nextWindow.document.body.appendChild(root);
    setPipWindow(nextWindow);
  };

  return (
    <>
      <div className="mini-status-host">
        {isSupported ? (
          <button
            className="pip-open-button mini-status is-launcher"
            type="button"
            aria-label="常時表示ステータスを開く"
            onClick={openPictureInPicture}
          >
            <span className="mini-collapsed">
              <span className="mini-arrow mini-arrow-right" aria-hidden="true" />
              <span className="mini-dot" style={{ backgroundColor: moodColor[mood] }} />
            </span>
          </button>
        ) : (
          <MiniStatusPanel mood={mood} onMoodChange={onMoodChange} />
        )}
      </div>
      {pipWindow &&
        createPortal(
          <MiniStatusPanel
            mood={mood}
            onMoodChange={onMoodChange}
            onExpandedChange={(expanded) => {
              try {
                pipWindow.resizeTo(pipWidth, expanded ? pipExpandedHeight : pipCollapsedHeight);
              } catch {
                // Some browsers keep Picture-in-Picture windows user-resizable only.
              }
            }}
            variant="pip"
          />,
          pipWindow.document.getElementById("pip-root") ?? pipWindow.document.body,
        )}
    </>
  );
}
