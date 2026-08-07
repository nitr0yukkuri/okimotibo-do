import React, { useState } from "react";
import "./StatusDisplay.css";
import { supabase } from "./supabase-client";

type Mood = "busy" | "available" | "neutral";

interface StatusDisplayProps {
  mood: Mood;
}

export function StatusDisplay({ mood: initialMood }: StatusDisplayProps) {
  const [mood, setMood] = useState<Mood>(initialMood);

  const content = {
    busy: {
      icon: (
        <svg viewBox="0 0 100 100" className="status-icon">
          <path d="M20 20 L80 80 M80 20 L20 80" stroke="currentColor" strokeWidth="15" strokeLinecap="round" />
        </svg>
      ),
      title: "作業中",
      subtitle: "話しかけないでね",
    },
    available: {
      icon: (
        <svg viewBox="0 0 100 100" className="status-icon">
          <circle cx="50" cy="50" r="30" stroke="currentColor" strokeWidth="15" fill="none" />
        </svg>
      ),
      title: "ひま！",
      subtitle: "喋りたいです",
    },
    neutral: {
      icon: (
        <svg viewBox="0 0 100 100" className="status-icon">
          <path d="M50 20 L20 80 L80 80 Z" stroke="currentColor" strokeWidth="12" fill="none" strokeLinejoin="round" />
        </svg>
      ),
      title: "反応可能！",
      subtitle: "用事があればどうぞ",
    },
  };

  const current = content[mood];

  return (
    <div className={`status-display theme-${mood}`}>
      <div className="status-graphic">
        {current.icon}
      </div>
      <div className="status-text">
        <h1>{current.title}</h1>
        <p>{current.subtitle}</p>
      </div>
      <button className="status-logout" type="button" onClick={() => supabase?.auth.signOut()}>
          ログアウト
      </button>
    </div>
  );
}